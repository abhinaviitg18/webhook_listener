#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME="${MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME:-agenthook-mail-receipt-guard}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/agenthook/prod/env}"
APP_ENV_INLINE_B64="${APP_ENV_INLINE_B64:-}"
BUILD_DIR="${BUILD_DIR:-/tmp/agenthook-mail-receipt-guard}"
ZIP_PATH="${ZIP_PATH:-$BUILD_DIR/bootstrap.zip}"
LAMBDA_ARCH="${LAMBDA_ARCH:-arm64}"

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

if ! aws lambda get-function \
  --region "$AWS_REGION" \
  --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" >/dev/null 2>&1; then
  echo "Mail receipt guard Lambda ${MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME} does not exist in ${AWS_REGION}; skipping deploy."
  exit 0
fi

go test ./cmd/... ./internal/...

export CGO_ENABLED=0
GOOS=linux GOARCH="$LAMBDA_ARCH" go build -tags lambda.norpc -o "$BUILD_DIR/bootstrap" ./cmd/mailreceiptguardlambda
(cd "$BUILD_DIR" && zip -q -j "$ZIP_PATH" bootstrap)

aws lambda update-function-code \
  --region "$AWS_REGION" \
  --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" \
  --zip-file "fileb://$ZIP_PATH" >/dev/null

aws lambda wait function-updated --region "$AWS_REGION" --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME"

aws lambda update-function-configuration \
  --region "$AWS_REGION" \
  --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" \
  --runtime provided.al2023 \
  --handler bootstrap \
  --timeout 30 \
  --memory-size 512 \
  --environment "Variables={APP_ENV_SSM_PARAM=$APP_ENV_SSM_PARAM,APP_ENV_INLINE_B64=$APP_ENV_INLINE_B64}" >/dev/null

aws lambda wait function-updated --region "$AWS_REGION" --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME"
