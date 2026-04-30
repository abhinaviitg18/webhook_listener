package config

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	DefaultLambdaRegion       = "us-east-1"
	DefaultLambdaEnvParameter = "/agenthook/prod/env"
	InlineEnvVarName          = "APP_ENV_INLINE_B64"
	LambdaEnvParameterName    = "APP_ENV_SSM_PARAM"
	LambdaRegionEnvName       = "AWS_REGION"
)

// LoadLambdaRuntimeEnv hydrates env from the inline base64 blob first, then
// falls back to the configured SSM parameter when core app settings are still
// missing. This keeps Lambda entrypoints aligned without requiring local files.
func LoadLambdaRuntimeEnv(ctx context.Context) error {
	if err := loadInlineEnv(); err != nil {
		return err
	}
	if os.Getenv("TIDB_DSN") != "" || os.Getenv("COMMERCE_MYSQL_DSN") != "" || os.Getenv("SCALEKIT_BASE_URL") != "" {
		return nil
	}

	paramName := strings.TrimSpace(os.Getenv(LambdaEnvParameterName))
	if paramName == "" {
		paramName = DefaultLambdaEnvParameter
	}

	region := strings.TrimSpace(os.Getenv(LambdaRegionEnvName))
	if region == "" {
		region = DefaultLambdaRegion
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

	return ApplyEnvFileContents(*resp.Parameter.Value)
}

func loadInlineEnv() error {
	encoded := strings.TrimSpace(os.Getenv(InlineEnvVarName))
	if encoded == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode inline env: %w", err)
	}

	return ApplyEnvFileContents(string(decoded))
}

func ApplyEnvFileContents(contents string) error {
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
