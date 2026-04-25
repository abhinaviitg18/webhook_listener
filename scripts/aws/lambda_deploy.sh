#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
LAMBDA_FUNCTION_NAME="${LAMBDA_FUNCTION_NAME:-agenthook-app}"
LAMBDA_ROLE_ARN="${LAMBDA_ROLE_ARN:-}"
LAMBDA_ROLE_NAME="${LAMBDA_ROLE_NAME:-${LAMBDA_FUNCTION_NAME}-execution-role}"
LAMBDA_SOURCE_FUNCTION_NAME="${LAMBDA_SOURCE_FUNCTION_NAME:-processor}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/agenthook/prod/env}"
APP_ENV_INLINE_B64="${APP_ENV_INLINE_B64:-}"
LAMBDA_ARCHITECTURE="${LAMBDA_ARCHITECTURE:-x86_64}"
LAMBDA_MEMORY_SIZE="${LAMBDA_MEMORY_SIZE:-1024}"
LAMBDA_TIMEOUT="${LAMBDA_TIMEOUT:-30}"
LAMBDA_ENVIRONMENT="${LAMBDA_ENVIRONMENT:-production}"
LAMBDA_ORIGIN_SHARED_SECRET="${LAMBDA_ORIGIN_SHARED_SECRET:-}"
BUILD_DIR="${BUILD_DIR:-/tmp/agenthook-lambda}"
ZIP_PATH="${ZIP_PATH:-$BUILD_DIR/bootstrap.zip}"
TRUST_POLICY_PATH="${TRUST_POLICY_PATH:-$BUILD_DIR/lambda-trust-policy.json}"

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

resolve_lambda_role() {
  if [[ -n "$LAMBDA_ROLE_ARN" ]]; then
    return
  fi

  if [[ -n "$LAMBDA_SOURCE_FUNCTION_NAME" ]]; then
    local source_function_name source_role
    source_function_name="$(aws lambda list-functions --region "$AWS_REGION" \
      --query "Functions[?contains(FunctionName, '${LAMBDA_SOURCE_FUNCTION_NAME}')].FunctionName | [0]" \
      --output text 2>/dev/null || true)"
    if [[ -n "$source_function_name" && "$source_function_name" != "None" ]]; then
      source_role="$(aws lambda get-function-configuration --region "$AWS_REGION" \
        --function-name "$source_function_name" \
        --query 'Role' \
        --output text 2>/dev/null || true)"
      if [[ -n "$source_role" && "$source_role" != "None" ]]; then
        LAMBDA_ROLE_ARN="$source_role"
        export LAMBDA_ROLE_ARN
        echo "Reusing lambda role from ${source_function_name}" >&2
        return
      fi
    fi
  fi

  local account_id role_exists=0 ssm_resource_arn
  account_id="$(aws sts get-caller-identity --region "$AWS_REGION" --query 'Account' --output text)"
  ssm_resource_arn="arn:aws:ssm:${AWS_REGION}:${account_id}:parameter${APP_ENV_SSM_PARAM}"

  if aws iam get-role --role-name "$LAMBDA_ROLE_NAME" >/dev/null 2>&1; then
    role_exists=1
  fi

  if [[ "$role_exists" -eq 0 ]]; then
    cat > "$TRUST_POLICY_PATH" <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "Service": "lambda.amazonaws.com" },
      "Action": "sts:AssumeRole"
    }
  ]
}
JSON

    echo "Creating lambda IAM role..."
    aws iam create-role \
      --role-name "$LAMBDA_ROLE_NAME" \
      --assume-role-policy-document "file://$TRUST_POLICY_PATH" >/dev/null
  fi

  aws iam attach-role-policy \
    --role-name "$LAMBDA_ROLE_NAME" \
    --policy-arn arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole >/dev/null

  cat > "$BUILD_DIR/lambda-ssm-policy.json" <<JSON
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["ssm:GetParameter"],
      "Resource": ["${ssm_resource_arn}"]
    }
  ]
}
JSON

  aws iam put-role-policy \
    --role-name "$LAMBDA_ROLE_NAME" \
    --policy-name "${LAMBDA_FUNCTION_NAME}-ssm-env-read" \
    --policy-document "file://$BUILD_DIR/lambda-ssm-policy.json" >/dev/null

  LAMBDA_ROLE_ARN="$(aws iam get-role --role-name "$LAMBDA_ROLE_NAME" --query 'Role.Arn' --output text)"
  export LAMBDA_ROLE_ARN
  sleep 10
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

resolve_lambda_role

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
  echo "Creating lambda function..."
  aws lambda create-function \
    --region "$AWS_REGION" \
    --function-name "$LAMBDA_FUNCTION_NAME" \
    --runtime provided.al2023 \
    --handler bootstrap \
    --memory-size "$LAMBDA_MEMORY_SIZE" \
    --timeout "$LAMBDA_TIMEOUT" \
    --role "$LAMBDA_ROLE_ARN" \
    --zip-file "fileb://$ZIP_PATH" \
    --environment "Variables={APP_ENV_SSM_PARAM=$APP_ENV_SSM_PARAM,APP_ENV_INLINE_B64=$APP_ENV_INLINE_B64,LAMBDA_ORIGIN_SHARED_SECRET=$LAMBDA_ORIGIN_SHARED_SECRET,APP_RUNTIME_ENV=$LAMBDA_ENVIRONMENT}"
else
  lambda_wait_until_idle 30 10 || true

  echo "Updating lambda code..."
  aws lambda update-function-code \
    --region "$AWS_REGION" \
    --function-name "$LAMBDA_FUNCTION_NAME" \
    --zip-file "fileb://$ZIP_PATH" >/dev/null

  echo "Updating lambda configuration..."
  lambda_update_configuration_with_retry 18 10
fi

echo "Waiting for function to become active..."
aws lambda wait function-updated --region "$AWS_REGION" --function-name "$LAMBDA_FUNCTION_NAME"

if ! aws lambda get-function-url-config --region "$AWS_REGION" --function-name "$LAMBDA_FUNCTION_NAME" >/dev/null 2>&1; then
  echo "Creating function URL..."
  aws lambda create-function-url-config \
    --region "$AWS_REGION" \
    --function-name "$LAMBDA_FUNCTION_NAME" \
    --auth-type NONE >/dev/null
fi

aws lambda update-function-url-config \
  --region "$AWS_REGION" \
  --function-name "$LAMBDA_FUNCTION_NAME" \
  --auth-type NONE >/dev/null

aws lambda add-permission \
  --region "$AWS_REGION" \
  --function-name "$LAMBDA_FUNCTION_NAME" \
  --statement-id "${LAMBDA_FUNCTION_NAME}-function-url-public" \
  --action lambda:InvokeFunctionUrl \
  --principal '*' \
  --function-url-auth-type NONE >/dev/null 2>&1 || true

aws lambda add-permission \
  --region "$AWS_REGION" \
  --function-name "$LAMBDA_FUNCTION_NAME" \
  --statement-id "${LAMBDA_FUNCTION_NAME}-public-invoke" \
  --action lambda:InvokeFunction \
  --principal '*' \
  --invoked-via-function-url true >/dev/null 2>&1 || true

echo "Lambda function URL:"
aws lambda get-function-url-config \
  --region "$AWS_REGION" \
  --function-name "$LAMBDA_FUNCTION_NAME" \
  --query 'FunctionUrl' \
  --output text
