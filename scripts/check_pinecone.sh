#!/usr/bin/env bash
set -euo pipefail
if [[ -z "${PINECONE_API_KEY:-}" ]]; then
  echo "PINECONE_API_KEY missing"
  exit 1
fi

code=$(curl -sS -o /tmp/hookweb_pinecone_indexes.json -w "%{http_code}" \
  -H "Api-Key: ${PINECONE_API_KEY}" \
  -H "X-Pinecone-API-Version: 2024-07" \
  https://api.pinecone.io/indexes)

echo "pinecone_status=$code"
cat /tmp/hookweb_pinecone_indexes.json
