#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://app.agenthook.store}"
EMAIL="${EMAIL:-techhiring@agentmail.to}"
PROVIDER="${PROVIDER:-github}"
OUT_DIR="${OUT_DIR:-/tmp/agenthook-prod-smoke}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"

mkdir -p "$OUT_DIR"

json_get() {
  local file="$1"
  local expr="$2"
  python3 - "$file" "$expr" <<'PY'
import json, sys

path = sys.argv[2].split(".")
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)
cur = data
for part in path:
    if part == "":
        continue
    if isinstance(cur, list):
        cur = cur[int(part)]
    else:
        cur = cur.get(part)
print("" if cur is None else cur)
PY
}

api_call() {
  local name="$1"
  local method="$2"
  local url="$3"
  local body_file="$4"
  shift 4

  local response_file="$OUT_DIR/${name}.json"
  local status_file="$OUT_DIR/${name}.status"

  curl -sS -X "$method" "$url" \
    -H 'accept: application/json' \
    "$@" \
    ${body_file:+--data-binary @"$body_file"} \
    -o "$response_file" \
    -w '%{http_code}' > "$status_file"

  printf '%s' "$response_file"
}

assert_status() {
  local name="$1"
  local expected="$2"
  local actual
  actual="$(cat "$OUT_DIR/${name}.status")"
  if [[ "$actual" != "$expected" ]]; then
    echo "[$name] expected status $expected, got $actual"
    cat "$OUT_DIR/${name}.json"
    exit 1
  fi
}

write_json() {
  local file="$1"
  local payload="$2"
  printf '%s' "$payload" > "$file"
}

listener_id="prod-smoke-${RUN_ID}"
legacy_type="telegram-update-prod-smoke-${RUN_ID}"
event_id_one="evt_listener_${RUN_ID}_1"
event_id_two="evt_listener_${RUN_ID}_2"
legacy_event_id="evt_legacy_${RUN_ID}_1"

echo "[1/12] register prod token for $EMAIL"
register_body="$OUT_DIR/register-body.json"
write_json "$register_body" "{\"email\":\"$EMAIL\"}"
api_call "01-register" "POST" "$BASE_URL/api/register/email" "$register_body" -H 'content-type: application/json' >/dev/null
assert_status "01-register" "201"
TOKEN="$(json_get "$OUT_DIR/01-register.json" "token")"
ACCOUNT_SLUG="$(json_get "$OUT_DIR/01-register.json" "account.slug")"

if [[ -z "$TOKEN" || -z "$ACCOUNT_SLUG" ]]; then
  echo "registration did not return token or account slug"
  cat "$OUT_DIR/01-register.json"
  exit 1
fi

echo "[2/12] mint second prod API token"
api_call "02-create-api-token" "POST" "$BASE_URL/v1/auth/tokens" "" \
  -H "Authorization: Bearer $TOKEN" >/dev/null
assert_status "02-create-api-token" "201"
SECOND_TOKEN="$(json_get "$OUT_DIR/02-create-api-token.json" "token")"
if [[ -z "$SECOND_TOKEN" ]]; then
  echo "second token generation failed"
  cat "$OUT_DIR/02-create-api-token.json"
  exit 1
fi

echo "[3/12] create listener to reproduce webhook creation path"
listener_body="$OUT_DIR/listener-body.json"
write_json "$listener_body" "{\"provider\":\"$PROVIDER\",\"listener_id\":\"$listener_id\",\"deployment_mode\":\"normal_plan\",\"plain_text_action\":\"store_mysql\",\"use_llm_fallback\":false}"
api_call "03-create-listener" "POST" "$BASE_URL/v1/listeners" "$listener_body" \
  -H "Authorization: Bearer $SECOND_TOKEN" \
  -H 'content-type: application/json' >/dev/null
assert_status "03-create-listener" "201"
LISTENER_WEBHOOK="$(json_get "$OUT_DIR/03-create-listener.json" "webhook_url")"

echo "[4/12] list listeners"
api_call "04-list-listeners" "GET" "$BASE_URL/v1/listeners" "" \
  -H "Authorization: Bearer $SECOND_TOKEN" >/dev/null
assert_status "04-list-listeners" "200"

echo "[5/12] create second listener secret"
listener_secret_body="$OUT_DIR/listener-secret-body.json"
write_json "$listener_secret_body" "{\"provider\":\"$PROVIDER\"}"
api_call "05-create-listener-secret" "POST" "$BASE_URL/v1/listeners/$listener_id/secrets" "$listener_secret_body" \
  -H "Authorization: Bearer $SECOND_TOKEN" \
  -H 'content-type: application/json' >/dev/null
assert_status "05-create-listener-secret" "201"
SECOND_LISTENER_WEBHOOK="$(json_get "$OUT_DIR/05-create-listener-secret.json" "webhook_url")"

echo "[6/12] list listener secrets"
api_call "06-list-listener-secrets" "GET" "$BASE_URL/v1/listeners/$listener_id/secrets?provider=$PROVIDER" "" \
  -H "Authorization: Bearer $SECOND_TOKEN" >/dev/null
assert_status "06-list-listener-secrets" "200"

echo "[7/12] ingest first listener webhook"
listener_ingest_body="$OUT_DIR/listener-ingest-body.json"
write_json "$listener_ingest_body" "{\"event_id\":\"$event_id_one\",\"event_type\":\"push\",\"message\":{\"text\":\"prod smoke listener primary\"},\"source\":\"$PROVIDER\"}"
api_call "07-listener-ingest-primary" "POST" "$LISTENER_WEBHOOK" "$listener_ingest_body" \
  -H 'content-type: application/json' >/dev/null
assert_status "07-listener-ingest-primary" "202"

echo "[8/12] ingest second listener webhook"
listener_ingest_body_two="$OUT_DIR/listener-ingest-body-two.json"
write_json "$listener_ingest_body_two" "{\"event_id\":\"$event_id_two\",\"event_type\":\"push\",\"message\":{\"text\":\"prod smoke listener secondary\"},\"source\":\"$PROVIDER\"}"
api_call "08-listener-ingest-secondary" "POST" "$SECOND_LISTENER_WEBHOOK" "$listener_ingest_body_two" \
  -H 'content-type: application/json' >/dev/null
assert_status "08-listener-ingest-secondary" "202"

echo "[9/12] list listener events"
api_call "09-listener-events" "GET" "$BASE_URL/v1/listeners/$listener_id/events?provider=$PROVIDER&limit=20" "" \
  -H "Authorization: Bearer $SECOND_TOKEN" >/dev/null
assert_status "09-listener-events" "200"

echo "[10/12] create legacy type + secret"
legacy_type_body="$OUT_DIR/legacy-type-body.json"
write_json "$legacy_type_body" "{\"type_key\":\"$legacy_type\",\"plain_text_action\":\"store_mysql\",\"use_llm_fallback\":false}"
api_call "10-create-legacy-type" "POST" "$BASE_URL/api/webhooks/types" "$legacy_type_body" \
  -H "Authorization: Bearer $SECOND_TOKEN" \
  -H 'content-type: application/json' >/dev/null
assert_status "10-create-legacy-type" "201"

api_call "10b-list-legacy-types" "GET" "$BASE_URL/api/webhooks/types" "" \
  -H "Authorization: Bearer $SECOND_TOKEN" >/dev/null
assert_status "10b-list-legacy-types" "200"

legacy_secret_body="$OUT_DIR/legacy-secret-body.json"
write_json "$legacy_secret_body" "{\"type_key\":\"$legacy_type\"}"
api_call "10c-create-legacy-secret" "POST" "$BASE_URL/api/webhooks/secrets" "$legacy_secret_body" \
  -H "Authorization: Bearer $SECOND_TOKEN" \
  -H 'content-type: application/json' >/dev/null
assert_status "10c-create-legacy-secret" "201"
LEGACY_SECRET_ID="$(json_get "$OUT_DIR/10c-create-legacy-secret.json" "secret.id")"
LEGACY_WEBHOOK="$BASE_URL$(json_get "$OUT_DIR/10c-create-legacy-secret.json" "webhook_url")"

echo "[11/12] ingest legacy webhook and list account events"
legacy_ingest_body="$OUT_DIR/legacy-ingest-body.json"
write_json "$legacy_ingest_body" "{\"event_id\":\"$legacy_event_id\",\"message\":\"prod smoke legacy webhook\"}"
api_call "11-legacy-ingest" "POST" "$LEGACY_WEBHOOK" "$legacy_ingest_body" \
  -H 'content-type: application/json' >/dev/null
assert_status "11-legacy-ingest" "202"

api_call "11b-list-events" "GET" "$BASE_URL/api/events?limit=20" "" \
  -H "Authorization: Bearer $SECOND_TOKEN" >/dev/null
assert_status "11b-list-events" "200"

echo "[12/12] revoke legacy secret and verify unauthorized reuse"
api_call "12-delete-legacy-secret" "DELETE" "$BASE_URL/api/webhooks/secrets/$LEGACY_SECRET_ID" "" \
  -H "Authorization: Bearer $SECOND_TOKEN" >/dev/null
assert_status "12-delete-legacy-secret" "200"

api_call "12b-legacy-ingest-revoked" "POST" "$LEGACY_WEBHOOK" "$legacy_ingest_body" \
  -H 'content-type: application/json' >/dev/null
assert_status "12b-legacy-ingest-revoked" "401"

echo "prod smoke test passed"
echo "account_slug=$ACCOUNT_SLUG"
echo "listener_id=$listener_id"
echo "legacy_type=$legacy_type"
echo "artifacts=$OUT_DIR"
