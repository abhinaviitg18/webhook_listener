#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${SCALEKIT_BASE_URL:-}" && -f ".env" ]]; then
  SCALEKIT_BASE_URL=$(awk -F= '/^SCALEKIT_BASE_URL=/{sub(/^SCALEKIT_BASE_URL=/,"");print;exit}' .env)
fi
if [[ -z "${SCALEKIT_API_KEY:-}" && -f ".env" ]]; then
  SCALEKIT_API_KEY=$(awk -F= '/^SCALEKIT_API_KEY=/{sub(/^SCALEKIT_API_KEY=/,"");print;exit}' .env)
fi

if [[ -z "${SCALEKIT_BASE_URL:-}" || -z "${SCALEKIT_API_KEY:-}" ]]; then
  echo "SCALEKIT_BASE_URL or SCALEKIT_API_KEY missing"
  exit 1
fi

code=$(curl -sS -o /tmp/agenthook_scalekit_health.json -w "%{http_code}" \
  -H "Authorization: Bearer ${SCALEKIT_API_KEY}" \
  "${SCALEKIT_BASE_URL%/}/.well-known/openid-configuration")

echo "scalekit_status=$code"
cat /tmp/agenthook_scalekit_health.json
