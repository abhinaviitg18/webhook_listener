#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
MAIL_INGRESS_LAMBDA_FUNCTION_NAME="${MAIL_INGRESS_LAMBDA_FUNCTION_NAME:-agenthook-mail-ingress}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/agenthook/prod/env}"
APP_ENV_INLINE_B64="${APP_ENV_INLINE_B64:-}"
BUILD_DIR="${BUILD_DIR:-/tmp/agenthook-mail-ingress}"
ZIP_PATH="${ZIP_PATH:-$BUILD_DIR/bootstrap.zip}"

mkdir -p "$BUILD_DIR"
rm -f "$BUILD_DIR/bootstrap" "$ZIP_PATH"

if [[ -z "$APP_ENV_INLINE_B64" && -n "$APP_ENV_SSM_PARAM" ]]; then
  APP_ENV_INLINE_B64="$(aws ssm get-parameter \
    --region "$AWS_REGION" \
    --name "$APP_ENV_SSM_PARAM" \
    --with-decryption \
    --query 'Parameter.Value' \
    --output text | base64 | tr -d '\n')"
fi

go test ./cmd/... ./internal/...

export CGO_ENABLED=0
GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o "$BUILD_DIR/bootstrap" ./cmd/mailingresslambda
(cd "$BUILD_DIR" && zip -q -j "$ZIP_PATH" bootstrap)

aws lambda update-function-code \
  --region "$AWS_REGION" \
  --function-name "$MAIL_INGRESS_LAMBDA_FUNCTION_NAME" \
  --zip-file "fileb://$ZIP_PATH" >/dev/null

aws lambda update-function-configuration \
  --region "$AWS_REGION" \
  --function-name "$MAIL_INGRESS_LAMBDA_FUNCTION_NAME" \
  --runtime provided.al2023 \
  --handler bootstrap \
  --timeout 60 \
  --memory-size 1024 \
  --environment "Variables={APP_ENV_SSM_PARAM=$APP_ENV_SSM_PARAM,APP_ENV_INLINE_B64=$APP_ENV_INLINE_B64}" >/dev/null

aws lambda wait function-updated --region "$AWS_REGION" --function-name "$MAIL_INGRESS_LAMBDA_FUNCTION_NAME"
