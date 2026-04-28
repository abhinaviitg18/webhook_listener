#!/bin/bash
set -euo pipefail

# API Test Script: Skills + Reprocessing
# Usage:
#   TOKEN=... ./scripts/test_skills_and_tags.sh
# Optional:
#   BASE=https://app.agenthook.store
#   TYPE_KEY=lis::provider::listener::multitenant
#   LISTENER_ID=my-listener
#   PROVIDER=github
#   LIMIT=20

BASE="${BASE:-https://app.agenthook.store}"
TOKEN="${TOKEN:-}"
TYPE_KEY="${TYPE_KEY:-}"
LISTENER_ID="${LISTENER_ID:-}"
PROVIDER="${PROVIDER:-}"
LIMIT="${LIMIT:-20}"

if [[ -z "$TOKEN" ]]; then
  echo "TOKEN is required. Example:"
  echo "  TOKEN=your_api_token ./scripts/test_skills_and_tags.sh"
  exit 1
fi

AUTH="Authorization: Bearer $TOKEN"

api_get() {
  local path="$1"
  curl --fail --silent --show-error -H "$AUTH" "$BASE$path"
}

api_post() {
  local path="$1"
  local body="${2:-}"
  if [[ -n "$body" ]]; then
    curl --fail --silent --show-error -X POST -H "$AUTH" -H "Content-Type: application/json" "$BASE$path" -d "$body"
    return
  fi
  curl --fail --silent --show-error -X POST -H "$AUTH" "$BASE$path"
}

json_pretty() {
  python3 -m json.tool
}

urlencode() {
  python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$1"
}

read -r -d '' SKILLS_JSON <<'EOF' || true
[
  {
    "skill_key": "marketing-detector",
    "skill_prompt": "If this message is a marketing, promotional, or advertising message such as discount offers, sale alerts, or brand campaigns, tag it as marketing. Summarize what the promotion is about in 1-2 sentences.",
    "match_contains": "",
    "forced_action": "store_mysql",
    "memory_write_mode": "none",
    "priority": 100,
    "enabled": true
  },
  {
    "skill_key": "otp-detector",
    "skill_prompt": "If this message contains an OTP, verification code, or login code, tag it as otp. Extract the code and the service it belongs to.",
    "match_contains": "",
    "forced_action": "store_mysql",
    "memory_write_mode": "none",
    "priority": 90,
    "enabled": true
  },
  {
    "skill_key": "transaction-alert",
    "skill_prompt": "If this message is a transactional alert such as a bank transaction, payment confirmation, delivery update, or booking confirmation, tag it as transactional. Extract key details like amount, merchant, and status.",
    "match_contains": "",
    "forced_action": "store_mysql",
    "memory_write_mode": "none",
    "priority": 85,
    "enabled": true
  },
  {
    "skill_key": "personal-message",
    "skill_prompt": "If this is a personal message from a known contact, tag it as personal. Summarize who sent it and what they said.",
    "match_contains": "",
    "forced_action": "store_mysql",
    "memory_write_mode": "update_or_insert",
    "priority": 80,
    "enabled": true
  },
  {
    "skill_key": "newsletter-detector",
    "skill_prompt": "If this is a newsletter, digest, or informational broadcast from a service, tag it as newsletter. Summarize the key topics covered.",
    "match_contains": "",
    "forced_action": "store_mysql",
    "memory_write_mode": "none",
    "priority": 70,
    "enabled": true
  }
]
EOF

echo "=== 1. Checking account listeners ==="
LISTENERS_JSON="$(api_get "/v1/listeners")"
echo "$LISTENERS_JSON" | json_pretty

if [[ -z "$TYPE_KEY" ]]; then
  TYPE_KEY="$(python3 - "$LISTENERS_JSON" "$LISTENER_ID" "$PROVIDER" <<'PY'
import json, sys

listeners = json.loads(sys.argv[1])
listener_id = sys.argv[2].strip()
provider = sys.argv[3].strip()

for item in listeners:
    if listener_id and item.get("listener_id") != listener_id:
        continue
    if provider and item.get("provider") != provider:
        continue
    type_key = (item.get("type_key") or "").strip()
    if type_key:
        print(type_key)
        raise SystemExit(0)

raise SystemExit(1)
PY
)"
fi

if [[ -z "$TYPE_KEY" ]]; then
  echo "Unable to determine TYPE_KEY. Set TYPE_KEY or narrow with LISTENER_ID/PROVIDER."
  exit 1
fi

echo ""
echo "Resolved TYPE_KEY: $TYPE_KEY"

echo ""
echo "=== 2. Checking existing skills for type_key: $TYPE_KEY ==="
ENCODED_TYPE_KEY="$(urlencode "$TYPE_KEY")"
EXISTING_SKILLS_JSON="$(api_get "/api/policy/skills?type_key=$ENCODED_TYPE_KEY")"
echo "$EXISTING_SKILLS_JSON" | json_pretty

echo ""
echo "=== 3. Creating any missing skills ==="
python3 - "$SKILLS_JSON" "$EXISTING_SKILLS_JSON" "$TYPE_KEY" > /tmp/agenthook_skill_payloads.json <<'PY'
import json, sys

skills = json.loads(sys.argv[1])
existing = json.loads(sys.argv[2])
type_key = sys.argv[3]
existing_keys = {item.get("skill_key") for item in existing}

payloads = []
for skill in skills:
    if skill["skill_key"] in existing_keys:
        continue
    payload = dict(skill)
    payload["type_key"] = type_key
    payloads.append(payload)

print(json.dumps(payloads))
PY

MISSING_COUNT="$(python3 -c 'import json; print(len(json.load(open("/tmp/agenthook_skill_payloads.json"))))')"
echo "Missing skills to create: $MISSING_COUNT"

python3 - "/tmp/agenthook_skill_payloads.json" <<'PY' | while IFS=$'\t' read -r skill_key payload; do
import json, sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    payloads = json.load(fh)

for payload in payloads:
    print(f"{payload['skill_key']}\t{json.dumps(payload)}")
PY
  echo "--- Creating: $skill_key ---"
  api_post "/api/policy/skills" "$payload" | json_pretty
done

echo ""
echo "=== 4. Verifying skills ==="
VERIFIED_SKILLS_JSON="$(api_get "/api/policy/skills?type_key=$ENCODED_TYPE_KEY")"
echo "$VERIFIED_SKILLS_JSON" | json_pretty

echo ""
echo "=== 5. Fetching last $LIMIT events ==="
EVENTS_JSON="$(api_get "/api/events?limit=$LIMIT")"
EVENT_IDS="$(python3 - "$EVENTS_JSON" "$TYPE_KEY" <<'PY'
import json, sys

events = json.loads(sys.argv[1])
type_key = sys.argv[2].strip()

for event in events:
    if type_key and event.get("type_key") != type_key:
        continue
    event_id = (event.get("id") or "").strip()
    if event_id:
        print(event_id)
PY
)"

EVENT_COUNT="$(printf '%s\n' "$EVENT_IDS" | sed '/^$/d' | wc -l | tr -d ' ')"
echo "Found $EVENT_COUNT matching events to reprocess"

if [[ "$EVENT_COUNT" -eq 0 ]]; then
  echo "No events matched TYPE_KEY=$TYPE_KEY in the last $LIMIT events."
  exit 1
fi

echo ""
echo "=== 6. Reprocessing each event ==="
SUCCESS_COUNT=0
while IFS= read -r EVENT_ID; do
  [[ -z "$EVENT_ID" ]] && continue
  echo "Reprocessing: $EVENT_ID"
  RESULT="$(api_post "/api/events/$EVENT_ID/re-run")"
  python3 - "$RESULT" <<'PY'
import json, sys

data = json.loads(sys.argv[1])
decision = data.get("decision", {})
tags = decision.get("tags", [])
summary = (decision.get("processed_text", "") or "").replace("\n", " ")[:120]
print(f"  Tags: {tags}")
print(f"  Summary: {summary}...")
PY
  SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
  echo "---"
done <<< "$EVENT_IDS"

echo ""
echo "=== 7. Testing tag filter API ==="
for TAG in marketing personal otp transactional newsletter; do
  TAG_LABEL="$(printf '%s' "$TAG" | python3 -c 'import sys; text = sys.stdin.read().strip(); print(text[:1].upper() + text[1:])')"
  echo "--- $TAG_LABEL events ---"
  TAG_RESULTS="$(api_get "/api/events/by-tag?tag=$TAG&limit=5")"
  python3 - "$TAG_RESULTS" <<'PY'
import json, sys

events = json.loads(sys.argv[1]) or []
print(f"Found {len(events)} events")
for event in events:
    body = ""
    try:
        payload = json.loads(event.get("payload_json", "{}"))
        body = (payload.get("body", "") or payload.get("text", ""))[:80]
    except Exception:
        pass
    print(f"  - {event.get('id', '')[:8]}... tags={event.get('tags_json', '')} body={body}")
PY
done

echo ""
echo "=== DONE ==="
echo "Reprocessed $SUCCESS_COUNT event(s) for TYPE_KEY=$TYPE_KEY"
