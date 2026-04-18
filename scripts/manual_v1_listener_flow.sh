#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:19082}"
EMAIL="${EMAIL:-techhiring@agentmail.to}"
OUT_DIR="${OUT_DIR:-/tmp/hookweb-v1-snapshots}"

mkdir -p "$OUT_DIR"

echo "[1/6] register account"
curl -sS -X POST "$BASE_URL/api/register/email" \
  -H "content-type: application/json" \
  -d "{\"email\":\"$EMAIL\"}" | tee "$OUT_DIR/01-register.json" >/dev/null
TOKEN=$(sed -n 's/.*"token":"\([^"]*\)".*/\1/p' "$OUT_DIR/01-register.json")
SLUG=$(sed -n 's/.*"slug":"\([^"]*\)".*/\1/p' "$OUT_DIR/01-register.json")

echo "[2/6] create listener"
curl -sS -X POST "$BASE_URL/v1/listeners" \
  -H "Authorization: Bearer $TOKEN" \
  -H "content-type: application/json" \
  -d '{"provider":"agentmail","deployment_mode":"normal_plan","plain_text_action":"store_mysql","use_llm_fallback":false}' | tee "$OUT_DIR/02-create-listener.json" >/dev/null
LISTENER_ID=$(sed -n 's/.*"listener_id":"\([^"]*\)".*/\1/p' "$OUT_DIR/02-create-listener.json")
WEBHOOK_URL=$(sed -n 's/.*"webhook_url":"\([^"]*\)".*/\1/p' "$OUT_DIR/02-create-listener.json")

echo "[3/6] ingest provider payload"
curl -sS -X POST "$BASE_URL$WEBHOOK_URL" \
  -H "content-type: application/json" \
  -d '{"event_id":"evt_snap_1","event_type":"inbox.message.received","message":{"subject":"Webhook Snapshot","text":"Testing raw and processed event storage"},"source":"agentmail"}' | tee "$OUT_DIR/03-ingest.json" >/dev/null

echo "[4/6] create second secret"
curl -sS -X POST "$BASE_URL/v1/listeners/$LISTENER_ID/secrets" \
  -H "Authorization: Bearer $TOKEN" \
  -H "content-type: application/json" \
  -d '{"provider":"agentmail"}' | tee "$OUT_DIR/04-create-secret.json" >/dev/null
SECOND_WEBHOOK_URL=$(sed -n 's/.*"webhook_url":"\([^"]*\)".*/\1/p' "$OUT_DIR/04-create-secret.json")

echo "[4b] ingest payload on second secret"
curl -sS -X POST "$BASE_URL$SECOND_WEBHOOK_URL" \
  -H "content-type: application/json" \
  -d '{"event_id":"evt_snap_2","event_type":"inbox.message.received","message":{"subject":"Webhook Snapshot Second Secret","text":"Testing group by secret view"},"source":"agentmail"}' | tee "$OUT_DIR/04b-ingest-second-secret.json" >/dev/null

echo "[5/6] list listeners"
curl -sS "$BASE_URL/v1/listeners" \
  -H "Authorization: Bearer $TOKEN" | tee "$OUT_DIR/05-list-listeners.json" >/dev/null

echo "[6/6] list listener events"
curl -sS "$BASE_URL/v1/listeners/$LISTENER_ID/events?provider=agentmail&limit=20" \
  -H "Authorization: Bearer $TOKEN" | tee "$OUT_DIR/06-list-events.json" >/dev/null

echo "done"
echo "snapshot_dir=$OUT_DIR"
echo "slug=$SLUG listener_id=$LISTENER_ID"
