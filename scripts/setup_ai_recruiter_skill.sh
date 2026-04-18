#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="${EMAIL:-7204909316@agentmail.to}"

REG=$(curl -sS -X POST "$BASE_URL/api/register/email" -H 'content-type: application/json' -d "{\"email\":\"$EMAIL\"}")
TOKEN=$(echo "$REG" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [[ -z "${TOKEN:-}" ]]; then
  echo "failed to get token"
  exit 1
fi

auth_post() {
  curl -sS -X POST "$1" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d "$2"
}

echo "creating deterministic type/signature/transform for AI Recruiter inbox webhook..."
auth_post "$BASE_URL/api/webhooks/types" '{"type_key":"ai-recruiter-inbox-message","plain_text_action":"store_mysql","use_llm_fallback":false}' >/dev/null || true
auth_post "$BASE_URL/api/resolver/signatures" '{"type_key":"ai-recruiter-inbox-message","version":1,"required_keys":["$.event_type","$.message.subject","$.recruiter.user_id","$.inbox.email"],"shape_hints":{"$.message":"object","$.recruiter":"object","$.inbox":"object"},"header_hints":{"x-htc-webhook-event-type":"inbox.message.received"},"confidence_threshold":0.7,"enabled":true,"source":"ai_recruiter"}' >/dev/null
auth_post "$BASE_URL/api/resolver/transforms" '{"type_key":"ai-recruiter-inbox-message","version":1,"engine":"dsl","dsl_text":"{\"extract\":{\"event_id\":\"$.event_id\",\"event_type\":\"$.event_type\",\"alias_email\":\"$.inbox.email\",\"from_email\":\"$.message.from_email\",\"to_email\":\"$.message.to_email\",\"subject\":\"$.message.subject\",\"recruiter_email\":\"$.recruiter.email\",\"org_id\":\"$.recruiter.org_id\",\"user_id\":\"$.recruiter.user_id\"},\"drop_null\":true}","deterministic_tests_json":"[]","status":"active"}' >/dev/null

echo "creating master policy + skill..."
auth_post "$BASE_URL/api/policy/master" '{"prompt_text":"For inbox.message.received use deterministic skill logic first. Store only compact canonical fields in memory. Skip memory write for empty subjects or auto-generated notifications.","updated_by":"setup_ai_recruiter_skill.sh"}' >/dev/null
auth_post "$BASE_URL/api/policy/skills" '{"type_key":"ai-recruiter-inbox-message","skill_key":"store-inbox-message","skill_prompt":"Store recruiter inbox message in mysql and keep compact memory.","match_contains":"inbox.message.received,message,subject,recruiter","forced_action":"store_mysql","memory_write_mode":"update_or_insert","priority":1,"enabled":true}' >/dev/null
auth_post "$BASE_URL/api/policy/skills" '{"type_key":"ai-recruiter-inbox-message","skill_key":"drop-empty-subject","skill_prompt":"Ignore events with blank subject to reduce noise.","match_contains":"inbox.message.received","forced_action":"no_action","memory_write_mode":"none","priority":2,"enabled":true}' >/dev/null

echo "done"
