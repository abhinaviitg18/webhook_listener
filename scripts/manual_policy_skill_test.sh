#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${EMAIL:-7204909316@agentmail.to}"

echo "register"
REG=$(curl -sS -X POST "$BASE_URL/api/register/email" -H 'content-type: application/json' -d "{\"email\":\"$EMAIL\"}")
TOKEN=$(echo "$REG" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
SLUG=$(echo "$REG" | sed -n 's/.*"slug":"\([^"]*\)".*/\1/p')

auth_json() {
  curl -sS -X POST "$1" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d "$2"
}

echo "create type + secret"
auth_json "$BASE_URL/api/webhooks/types" '{"type_key":"generic-json","plain_text_action":"","use_llm_fallback":true}' >/dev/null
SEC=$(auth_json "$BASE_URL/api/webhooks/secrets" '{"type_key":"generic-json"}')
SECRET=$(echo "$SEC" | sed -n 's/.*"secret_value":"\([^"]*\)".*/\1/p')

echo "set master prompt + skills"
auth_json "$BASE_URL/api/policy/master" '{"prompt_text":"Use per-type skills first; store only useful context in memory.","updated_by":"manual-suite"}' >/dev/null
auth_json "$BASE_URL/api/policy/skills" '{"type_key":"generic-json","skill_key":"drop-heartbeat","skill_prompt":"ignore heartbeat metrics","match_contains":"heartbeat,metrics","forced_action":"no_action","memory_write_mode":"none","priority":1,"enabled":true}' >/dev/null
auth_json "$BASE_URL/api/policy/skills" '{"type_key":"generic-json","skill_key":"incident-upsert","skill_prompt":"incident should be stored","match_contains":"incident,sev","forced_action":"store_mysql","memory_write_mode":"update_or_insert","priority":2,"enabled":true}' >/dev/null
auth_json "$BASE_URL/api/policy/skills" '{"type_key":"generic-json","skill_key":"event-insert-only","skill_prompt":"audit events should append","match_contains":"audit,event","forced_action":"store_mysql","memory_write_mode":"insert_only","priority":3,"enabled":true}' >/dev/null

echo "post 6 payload variants"
for PAYLOAD in \
'{"source":"github","event":"push","repo":"org/repo","commit":"abc"}' \
'{"source":"stripe","event":"payment_intent.succeeded","amount":3500}' \
'{"source":"slack","event":"message","team":"T123"}' \
'{"source":"telegram","event":"update","chat_id":"1001"}' \
'{"event":"heartbeat","kind":"metrics","cpu":0.3}' \
'{"event":"incident","sev":"sev1","ticket":"INC-22"}'
do
  curl -sS -X POST "$BASE_URL/url/$SLUG/$SECRET" -H 'content-type: application/json' -d "$PAYLOAD"
  echo
done

echo "recent events"
curl -sS "$BASE_URL/api/events?limit=20" -H "Authorization: Bearer $TOKEN"
echo
