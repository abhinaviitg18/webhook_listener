#!/usr/bin/env bash
set -euo pipefail

: "${RAILWAY_TOKEN:?RAILWAY_TOKEN is required}"

if [[ $# -lt 1 ]]; then
  echo "usage: $0 '<graphql-query>' [json-variables]" >&2
  exit 1
fi

query="$1"
variables="${2:-{}}"

curl -sS https://backboard.railway.com/graphql/v2 \
  -H 'Content-Type: application/json' \
  -H "Project-Access-Token: ${RAILWAY_TOKEN}" \
  --data "$(jq -cn --arg query "$query" --argjson variables "$variables" '{query:$query,variables:$variables}')"
