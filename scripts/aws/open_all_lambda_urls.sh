#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
PAGE_SIZE="${PAGE_SIZE:-50}"
SLEEP_SEC="${SLEEP_SEC:-2}"
CORS_CONFIG="${CORS_CONFIG:-AllowCredentials=true,AllowHeaders=*,AllowMethods=*,AllowOrigins=*,ExposeHeaders=*,MaxAge=86400}"

list_functions() {
  local marker=""
  while true; do
    local args=(lambda list-functions --region "$AWS_REGION" --max-items "$PAGE_SIZE")
    if [[ -n "$marker" ]]; then
      args+=(--starting-token "$marker")
    fi

    local payload
    payload="$(aws "${args[@]}")"
    printf '%s\n' "$payload" | python3 -c 'import json,sys; data=json.load(sys.stdin); [print(f["FunctionName"]) for f in data.get("Functions", [])]'

    marker="$(printf '%s\n' "$payload" | python3 -c 'import json,sys; data=json.load(sys.stdin); print(data.get("NextMarker",""))')"
    if [[ -z "$marker" || "$marker" == "None" ]]; then
      break
    fi
  done
}

ensure_public_url() {
  local function_name="$1"

  if ! aws lambda get-function-url-config --region "$AWS_REGION" --function-name "$function_name" >/dev/null 2>&1; then
    echo "Creating function URL for ${function_name}"
    aws lambda create-function-url-config \
      --region "$AWS_REGION" \
      --function-name "$function_name" \
      --auth-type NONE \
      --cors "$CORS_CONFIG" >/dev/null
  fi

  aws lambda update-function-url-config \
    --region "$AWS_REGION" \
    --function-name "$function_name" \
    --auth-type NONE \
    --cors "$CORS_CONFIG" >/dev/null

  aws lambda add-permission \
    --region "$AWS_REGION" \
    --function-name "$function_name" \
    --statement-id "${function_name}-function-url-public" \
    --action lambda:InvokeFunctionUrl \
    --principal '*' \
    --function-url-auth-type NONE >/dev/null 2>&1 || true

  aws lambda add-permission \
    --region "$AWS_REGION" \
    --function-name "$function_name" \
    --statement-id "${function_name}-public-invoke" \
    --action lambda:InvokeFunction \
    --principal '*' \
    --invoked-via-function-url true >/dev/null 2>&1 || true

  local url
  url="$(aws lambda get-function-url-config \
    --region "$AWS_REGION" \
    --function-name "$function_name" \
    --query 'FunctionUrl' \
    --output text)"

  printf '%s %s\n' "$function_name" "$url"
}

while IFS= read -r function_name; do
  [[ -z "$function_name" ]] && continue
  ensure_public_url "$function_name"
  sleep "$SLEEP_SEC"
done < <(list_functions)
