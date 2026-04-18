#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${EMAIL:-7204909316@agentmail.to}"

echo "register"
REG=$(curl -sS -X POST "$BASE_URL/api/register/email" -H 'content-type: application/json' -d "{\"email\":\"$EMAIL\"}")
TOKEN=$(echo "$REG" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

TYPE=$(curl -sS -X POST "$BASE_URL/api/webhooks/types" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"github-push","plain_text_action":"store_mysql","use_llm_fallback":true}')
curl -sS -X POST "$BASE_URL/api/webhooks/types" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"stripe-payment","plain_text_action":"store_mysql","use_llm_fallback":true}' >/dev/null
curl -sS -X POST "$BASE_URL/api/webhooks/types" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"slack-event","plain_text_action":"store_mysql","use_llm_fallback":true}' >/dev/null
SEC=$(curl -sS -X POST "$BASE_URL/api/webhooks/secrets" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"github-push"}')
SECRET=$(echo "$SEC" | sed -n 's/.*"secret_value":"\([^"]*\)".*/\1/p')
SLUG=$(echo "$REG" | sed -n 's/.*"slug":"\([^"]*\)".*/\1/p')

echo "create signatures"
for SIG in \
'{"type_key":"github-push","version":1,"required_keys":["$.repository.full_name","$.head_commit.id"],"shape_hints":{"$.repository":"object","$.head_commit":"object"},"header_hints":{},"confidence_threshold":0.7,"enabled":true,"source":"manual"}' \
'{"type_key":"stripe-payment","version":1,"required_keys":["$.type","$.data.object.amount"],"shape_hints":{"$.data":"object"},"header_hints":{},"confidence_threshold":0.7,"enabled":true,"source":"manual"}' \
'{"type_key":"slack-event","version":1,"required_keys":["$.team_id","$.event.type"],"shape_hints":{"$.event":"object"},"header_hints":{},"confidence_threshold":0.7,"enabled":true,"source":"manual"}'
do
  curl -sS -X POST "$BASE_URL/api/resolver/signatures" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d "$SIG" >/dev/null
 done

echo "create transforms"
curl -sS -X POST "$BASE_URL/api/resolver/transforms" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"github-push","version":1,"engine":"dsl","dsl_text":"{\"extract\":{\"repo\":\"$.repository.full_name\",\"commit\":\"$.head_commit.id\"}}","status":"active"}' >/dev/null
curl -sS -X POST "$BASE_URL/api/resolver/transforms" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"stripe-payment","version":1,"engine":"dsl","dsl_text":"{\"extract\":{\"event\":\"$.type\",\"amount\":\"$.data.object.amount\"}}","status":"active"}' >/dev/null
curl -sS -X POST "$BASE_URL/api/resolver/transforms" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d '{"type_key":"slack-event","version":1,"engine":"dsl","dsl_text":"{\"extract\":{\"team\":\"$.team_id\",\"event_type\":\"$.event.type\"}}","status":"active"}' >/dev/null

echo "post github payload"
GITHUB=$(curl -sS -X POST "$BASE_URL/url/$SLUG/$SECRET" -H 'content-type: application/json' -d '{"repository":{"full_name":"org/repo"},"head_commit":{"id":"abc123"}}')

echo "post stripe payload"
STRIPE=$(curl -sS -X POST "$BASE_URL/url/$SLUG/$SECRET" -H 'content-type: application/json' -d '{"type":"payment_intent.succeeded","data":{"object":{"amount":3500}}}')

echo "post slack payload"
SLACK=$(curl -sS -X POST "$BASE_URL/url/$SLUG/$SECRET" -H 'content-type: application/json' -d '{"team_id":"T123","event":{"type":"message","text":"hello"}}')

echo "post unknown payload (llm path)"
UNKNOWN=$(curl -sS -X POST "$BASE_URL/url/$SLUG/$SECRET" -H 'content-type: application/json' -d '{"foo":"bar","random":42}')

EVENTS=$(curl -sS "$BASE_URL/api/events?limit=10" -H "Authorization: Bearer $TOKEN")

echo "github => $GITHUB"
echo "stripe => $STRIPE"
echo "slack => $SLACK"
echo "unknown => $UNKNOWN"
echo "events => $EVENTS"
