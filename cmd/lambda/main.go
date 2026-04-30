package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"agenthook.store/internal/app"
	"agenthook.store/internal/config"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

const (
	originSecretHeader     = "x-agenthook-origin-secret"
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
		if err := config.LoadLambdaRuntimeEnv(ctx); err != nil {
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
