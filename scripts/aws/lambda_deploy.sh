#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
LAMBDA_FUNCTION_NAME="${LAMBDA_FUNCTION_NAME:-agenthook-app}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/agenthook/prod/env}"
APP_ENV_INLINE_B64="${APP_ENV_INLINE_B64:-}"
LAMBDA_ARCHITECTURE="${LAMBDA_ARCHITECTURE:-x86_64}"
LAMBDA_MEMORY_SIZE="${LAMBDA_MEMORY_SIZE:-1024}"
LAMBDA_TIMEOUT="${LAMBDA_TIMEOUT:-30}"
LAMBDA_ENVIRONMENT="${LAMBDA_ENVIRONMENT:-production}"
LAMBDA_ORIGIN_SHARED_SECRET="${LAMBDA_ORIGIN_SHARED_SECRET:-}"
BUILD_DIR="${BUILD_DIR:-/tmp/agenthook-lambda}"
ZIP_PATH="${ZIP_PATH:-$BUILD_DIR/bootstrap.zip}"

mkdir -p "$BUILD_DIR"
rm -f "$BUILD_DIR/bootstrap" "$ZIP_PATH"

lambda_wait_until_idle() {
  local attempts="${1:-20}"
  local sleep_sec="${2:-10}"
  local state last_status

  for ((i=1; i<=attempts; i++)); do
    state="$(aws lambda get-function-configuration \
      --region "$AWS_REGION" \
      --function-name "$LAMBDA_FUNCTION_NAME" \
      --query 'State' \
      --output text 2>/dev/null || true)"
    last_status="$(aws lambda get-function-configuration \
      --region "$AWS_REGION" \
      --function-name "$LAMBDA_FUNCTION_NAME" \
      --query 'LastUpdateStatus' \
      --output text 2>/dev/null || true)"

    if [[ "$state" == "Active" && "$last_status" != "InProgress" ]]; then
      return 0
    fi

    sleep "$sleep_sec"
  done

  return 1
}

lambda_update_configuration_with_retry() {
  local attempts="${1:-12}"
  local sleep_sec="${2:-10}"
  local output

  for ((i=1; i<=attempts; i++)); do
    if output="$(aws lambda update-function-configuration \
      --region "$AWS_REGION" \
      --function-name "$LAMBDA_FUNCTION_NAME" \
      --runtime provided.al2023 \
      --handler bootstrap \
      --memory-size "$LAMBDA_MEMORY_SIZE" \
      --timeout "$LAMBDA_TIMEOUT" \
      --environment "Variables={APP_ENV_SSM_PARAM=$APP_ENV_SSM_PARAM,APP_ENV_INLINE_B64=$APP_ENV_INLINE_B64,LAMBDA_ORIGIN_SHARED_SECRET=$LAMBDA_ORIGIN_SHARED_SECRET,APP_RUNTIME_ENV=$LAMBDA_ENVIRONMENT}" 2>&1)"; then
      return 0
    fi

    if [[ "$output" == *"ResourceConflictException"* ]]; then
      sleep "$sleep_sec"
      continue
    fi

    echo "$output" >&2
    return 1
  done

  echo "$output" >&2
  return 1
}

echo "Building frontend..."
(cd web && npm install && npm run build && mkdir -p ../internal/ui/dist && cp -r dist/* ../internal/ui/dist/)

echo "Running tests..."
go test ./cmd/... ./internal/...

echo "Building lambda binary..."
export CGO_ENABLED=0
GOARCH_VALUE="amd64"
if [[ "$LAMBDA_ARCHITECTURE" == "arm64" ]]; then
  GOARCH_VALUE="arm64"
fi
GOOS=linux GOARCH="$GOARCH_VALUE" go build -tags lambda.norpc -o "$BUILD_DIR/bootstrap" ./cmd/lambda

echo "Packaging lambda..."
(cd "$BUILD_DIR" && zip -q -j "$ZIP_PATH" bootstrap)

function_exists=0
if aws lambda get-function --region "$AWS_REGION" --function-name "$LAMBDA_FUNCTION_NAME" >/dev/null 2>&1; then
  function_exists=1
fi

if [[ -z "$APP_ENV_INLINE_B64" && -n "$APP_ENV_SSM_PARAM" ]]; then
  APP_ENV_INLINE_B64="$(aws ssm get-parameter \
    --region "$AWS_REGION" \
    --name "$APP_ENV_SSM_PARAM" \
    --with-decryption \
    --query 'Parameter.Value' \
    --output text | base64 | tr -d '\n')"
fi

if [[ "$function_exists" -eq 0 ]]; then
  echo "Lambda function ${LAMBDA_FUNCTION_NAME} does not exist; refusing to create it from CI." >&2
  exit 1
fi

lambda_wait_until_idle 30 10 || true

echo "Updating lambda code..."
aws lambda update-function-code \
  --region "$AWS_REGION" \
  --function-name "$LAMBDA_FUNCTION_NAME" \
  --zip-file "fileb://$ZIP_PATH" >/dev/null

echo "Updating lambda configuration..."
lambda_update_configuration_with_retry 18 10

echo "Waiting for function to become active..."
aws lambda wait function-updated --region "$AWS_REGION" --function-name "$LAMBDA_FUNCTION_NAME"

echo "Lambda function URL:"
aws lambda get-function-url-config \
  --region "$AWS_REGION" \
  --function-name "$LAMBDA_FUNCTION_NAME" \
  --query 'FunctionUrl' \
  --output text
