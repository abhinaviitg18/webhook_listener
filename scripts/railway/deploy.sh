#!/usr/bin/env bash
set -euo pipefail

if command -v railway >/dev/null 2>&1; then
  RAILWAY_BIN=(railway)
else
  RAILWAY_BIN=(npx -y @railway/cli@latest)
fi

: "${RAILWAY_TOKEN:?RAILWAY_TOKEN is required}"

declare -a scope_args=()
if [[ -n "${RAILWAY_PROJECT:-}" ]]; then
  scope_args+=("--project" "${RAILWAY_PROJECT}")
fi
if [[ -n "${RAILWAY_ENVIRONMENT:-}" ]]; then
  scope_args+=("--environment" "${RAILWAY_ENVIRONMENT}")
fi
if [[ -n "${RAILWAY_SERVICE:-}" ]]; then
  scope_args+=("--service" "${RAILWAY_SERVICE}")
fi

declare -a vars_to_sync=()

append_var() {
  local key="$1"
  local value="${!key-}"
  if [[ -z "${value}" ]]; then
    return
  fi
  vars_to_sync+=("${key}=${value}")
}

append_var APP_PLAN
append_var APP_DEPLOYMENT_MODE
append_var APP_SESSION_SECRET
append_var ALLOW_PUBLIC_REGISTRATION
append_var AUTOPROMOTE_ENABLED
append_var AUTOPROMOTE_MIN_CONFIDENCE
append_var AUTOPROMOTE_MIN_SUCCESS_RATE
append_var AUTOPROMOTE_SHADOW_TO_ACTIVE
append_var AUTOPROMOTE_VALIDATED_TO_SHADOW
append_var CEREBRAS_API_KEY
append_var CEREBRAS_BASE_URL
append_var CEREBRAS_MODEL
append_var COMMERCE_MYSQL_DSN
append_var DETERMINISTIC_ONLY_TYPE_KEYS
append_var GROQ_API_KEY
append_var GROQ_BASE_URL
append_var GROQ_MODEL
append_var LANGFUSE_ENABLED
append_var LANGFUSE_HOST
append_var LANGFUSE_PUBLIC_KEY
append_var LANGFUSE_SECRET_KEY
append_var LLM_API_KEY
append_var LLM_BASE_URL
append_var LLM_MODEL
append_var LLM_PROVIDER
append_var MAIL_DOMAIN
append_var OPENROUTER_API_KEY
append_var OPENROUTER_BASE_URL
append_var OPENROUTER_MODEL
append_var PINECONE_API_KEY
append_var PINECONE_ENABLED
append_var PINECONE_INDEX_URL
append_var PINECONE_NAMESPACE
append_var PUBLIC_BASE_URL
append_var SCALEKIT_API_KEY
append_var SCALEKIT_BASE_URL
append_var SCALEKIT_CLIENT_ID
append_var SCALEKIT_CLIENT_SECRET
append_var SCALEKIT_REDIRECT_URI
append_var SINGLE_TENANT_OWNER_ALIAS
append_var SINGLE_TENANT_OWNER_EMAIL
append_var SINGLE_TENANT_SETUP_TOKEN_SHA256
append_var TELEGRAM_BOT_TOKEN
append_var USE_IN_MEMORY_STORE
append_var VERIFY_HTC_SIGNATURE

if (( ${#vars_to_sync[@]} > 0 )); then
  echo "Syncing ${#vars_to_sync[@]} Railway variables"
  if (( ${#scope_args[@]} > 0 )); then
    "${RAILWAY_BIN[@]}" variable set "${scope_args[@]}" "${vars_to_sync[@]}"
  else
    "${RAILWAY_BIN[@]}" variable set "${vars_to_sync[@]}"
  fi
else
  echo "No Railway variables provided; skipping variable sync"
fi

echo "Deploying to Railway"
if (( ${#scope_args[@]} > 0 )); then
  "${RAILWAY_BIN[@]}" up --ci "${scope_args[@]}"
else
  "${RAILWAY_BIN[@]}" up --ci
fi
