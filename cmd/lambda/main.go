package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"agenthook.store/internal/app"
	"agenthook.store/internal/config"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

const (
	defaultRegion          = "us-east-1"
	defaultEnvParameter    = "/agenthook/prod/env"
	inlineEnvVarName       = "APP_ENV_INLINE_B64"
	originSecretHeader     = "x-agenthook-origin-secret"
	lambdaEnvParameterName = "APP_ENV_SSM_PARAM"
	lambdaRegionEnvName    = "AWS_REGION"
	lambdaOriginSecretEnv  = "LAMBDA_ORIGIN_SHARED_SECRET"
)

var (
	initOnce      sync.Once
	initErr       error
	lambdaAdapter *httpadapter.HandlerAdapterV2
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	if err := initRuntime(ctx); err != nil {
		log.Printf("lambda init failed: %v", err)
		return events.LambdaFunctionURLResponse{
			StatusCode: http.StatusInternalServerError,
			Headers: map[string]string{
				"content-type": "text/plain; charset=utf-8",
			},
			Body: "internal server error",
		}, nil
	}

	resp, err := lambdaAdapter.ProxyWithContext(ctx, toAPIGatewayV2Request(req))
	if err != nil {
		return events.LambdaFunctionURLResponse{}, err
	}

	return events.LambdaFunctionURLResponse{
		StatusCode:      resp.StatusCode,
		Headers:         resp.Headers,
		Body:            resp.Body,
		IsBase64Encoded: resp.IsBase64Encoded,
		Cookies:         resp.Cookies,
	}, nil
}

func initRuntime(ctx context.Context) error {
	initOnce.Do(func() {
		if err := loadEnvFromSSM(ctx); err != nil {
			initErr = err
			return
		}

		cfg := config.Load()
		if err := cfg.Validate(); err != nil {
			initErr = err
			return
		}

		router, err := app.BuildHTTPHandler(ctx, cfg)
		if err != nil {
			initErr = err
			return
		}

		lambdaAdapter = httpadapter.NewV2(withOriginSecret(router))
	})

	return initErr
}

func withOriginSecret(next http.Handler) http.Handler {
	sharedSecret := strings.TrimSpace(os.Getenv(lambdaOriginSecretEnv))
	if sharedSecret == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtleCompare(strings.TrimSpace(r.Header.Get(originSecretHeader)), sharedSecret) {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

func subtleCompare(actual, expected string) bool {
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func loadEnvFromSSM(ctx context.Context) error {
	if err := loadInlineEnv(); err != nil {
		return err
	}
	if os.Getenv("TIDB_DSN") != "" || os.Getenv("COMMERCE_MYSQL_DSN") != "" || os.Getenv("SCALEKIT_BASE_URL") != "" {
		return nil
	}

	paramName := strings.TrimSpace(os.Getenv(lambdaEnvParameterName))
	if paramName == "" {
		paramName = defaultEnvParameter
	}

	region := strings.TrimSpace(os.Getenv(lambdaRegionEnvName))
	if region == "" {
		region = defaultRegion
	}

	awsConfig, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}

	ssmClient := ssm.NewFromConfig(awsConfig)
	withDecryption := true
	resp, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &paramName,
		WithDecryption: &withDecryption,
	})
	if err != nil {
		return fmt.Errorf("get ssm parameter %s: %w", paramName, err)
	}
	if resp.Parameter == nil || resp.Parameter.Value == nil {
		return fmt.Errorf("ssm parameter %s was empty", paramName)
	}

	return applyEnvFile(*resp.Parameter.Value)
}

func loadInlineEnv() error {
	encoded := strings.TrimSpace(os.Getenv(inlineEnvVarName))
	if encoded == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode inline env: %w", err)
	}

	return applyEnvFile(string(decoded))
}

func applyEnvFile(contents string) error {
	scanner := bufio.NewScanner(strings.NewReader(contents))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid env line: %s", line)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("invalid env key in line: %s", line)
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan env file: %w", err)
	}

	return nil
}

func toAPIGatewayV2Request(req events.LambdaFunctionURLRequest) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		Version:               req.Version,
		RawPath:               req.RawPath,
		RawQueryString:        req.RawQueryString,
		Cookies:               req.Cookies,
		Headers:               req.Headers,
		QueryStringParameters: req.QueryStringParameters,
		Body:                  req.Body,
		IsBase64Encoded:       req.IsBase64Encoded,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			AccountID:    req.RequestContext.AccountID,
			RequestID:    req.RequestContext.RequestID,
			APIID:        req.RequestContext.APIID,
			DomainName:   req.RequestContext.DomainName,
			DomainPrefix: req.RequestContext.DomainPrefix,
			Time:         req.RequestContext.Time,
			TimeEpoch:    req.RequestContext.TimeEpoch,
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:    req.RequestContext.HTTP.Method,
				Path:      req.RequestContext.HTTP.Path,
				Protocol:  req.RequestContext.HTTP.Protocol,
				SourceIP:  req.RequestContext.HTTP.SourceIP,
				UserAgent: req.RequestContext.HTTP.UserAgent,
			},
		},
	}
}
