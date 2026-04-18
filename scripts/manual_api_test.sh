#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${EMAIL:-7204909316@agentmail.to}"

echo "1) register"
REG=$(curl -sS -X POST "$BASE_URL/api/register/email" -H 'content-type: application/json' -d "{\"email\":\"$EMAIL\"}")
TOKEN=$(echo "$REG" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

echo "2) create webhook type with plain text action"
TYPE=$(curl -sS -X POST "$BASE_URL/api/webhooks/types" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"telegram-update","plain_text_action":"forward_telegram","use_llm_fallback":true}')

echo "3) create telegram target (chat_id required)"
curl -sS -X POST "$BASE_URL/api/forward-targets" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"target_type":"telegram","config":{"chat_id":"123456"}}' >/dev/null

echo "4) create secret"
SEC=$(curl -sS -X POST "$BASE_URL/api/webhooks/secrets" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"telegram-update"}')
WEBHOOK=$(echo "$SEC" | sed -n 's/.*"webhook_url":"\([^"]*\)".*/\1/p')

echo "5) post webhook"
POST=$(curl -sS -X POST "$BASE_URL$WEBHOOK" -H 'content-type: application/json' -d '{"message":"candidate shortlisted","priority":"high"}')

echo "6) list events"
EVENTS=$(curl -sS "$BASE_URL/api/events?limit=10" -H "Authorization: Bearer $TOKEN")

echo "register => $REG"
echo "type => $TYPE"
echo "secret => $SEC"
echo "post => $POST"
echo "events => $EVENTS"
