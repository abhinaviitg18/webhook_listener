#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://app.agenthook.store}"
EMAIL="${EMAIL:-7204909316@agentmail.to}"
OUT_DIR="${OUT_DIR:-/tmp/agenthook-prod-rollout}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
PLAYWRIGHT_DIR="${PLAYWRIGHT_DIR:-$PWD/scripts/e2e}"
BROWSER_HEADLESS="${BROWSER_HEADLESS:-true}"
GITHUB_EVENTS="${GITHUB_EVENTS:-push}"
AGENTMAIL_EVENT_TYPES="${AGENTMAIL_EVENT_TYPES:-message.received}"

mkdir -p "$OUT_DIR"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "missing required env var: $name" >&2
    exit 1
  fi
}

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

json_escape() {
  python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$1"
}

api_call() {
  local name="$1"
  local method="$2"
  local url="$3"
  local body_file="${4:-}"
  shift 4 || true

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
    echo "[$name] expected status $expected, got $actual" >&2
    cat "$OUT_DIR/${name}.json" >&2
    exit 1
  fi
}

write_json() {
  local file="$1"
  local payload="$2"
  printf '%s' "$payload" > "$file"
}

gh_api() {
  gh api "$@"
}

ensure_playwright_deps() {
  require_cmd npm
  require_cmd node
  if [[ ! -d "$PLAYWRIGHT_DIR/node_modules" ]]; then
    npm --prefix "$PLAYWRIGHT_DIR" install >/dev/null
  fi
}

run_browser_helper() {
  local mode="$1"
  shift
  node "$PLAYWRIGHT_DIR/agenthook_prod_browser.mjs" "$mode" "$@"
}

poll_listener_events() {
  local provider="$1"
  local listener_id="$2"
  local needle="$3"
  local result_file="$4"
  local attempts="${5:-18}"

  for ((i=1; i<=attempts; i+=1)); do
    api_call "poll-${provider}-${listener_id}-${i}" "GET" \
      "$BASE_URL/v1/listeners/$listener_id/events?provider=$provider&limit=50" "" \
      -H "Authorization: Bearer $AGENTHOOK_TOKEN" >/dev/null
    assert_status "poll-${provider}-${listener_id}-${i}" "200"
    if python3 - "$OUT_DIR/poll-${provider}-${listener_id}-${i}.json" "$needle" <<'PY'
import json, sys
data = json.load(open(sys.argv[1], "r", encoding="utf-8"))
needle = sys.argv[2]
blob = json.dumps(data)
raise SystemExit(0 if needle in blob else 1)
PY
    then
      cp "$OUT_DIR/poll-${provider}-${listener_id}-${i}.json" "$result_file"
      return 0
    fi
    sleep 5
  done

  echo "timed out waiting for $provider listener event containing: $needle" >&2
  return 1
}

require_cmd curl
require_cmd python3
require_cmd gh
require_env AGENTMAIL_API_KEY

echo "[1/12] installing browser helper dependencies"
ensure_playwright_deps

echo "[2/12] authenticating in AgentHook via ScaleKit and minting API token"
export BASE_URL
run_browser_helper login-and-mint-token \
  --email "$EMAIL" \
  --out-dir "$OUT_DIR" \
  --headless "$BROWSER_HEADLESS" >"$OUT_DIR/browser-login.stdout.json"
AGENTHOOK_TOKEN="$(json_get "$OUT_DIR/browser-login.stdout.json" "token")"
STORAGE_STATE_PATH="$(json_get "$OUT_DIR/browser-login.stdout.json" "storage_state_path")"
if [[ -z "$AGENTHOOK_TOKEN" || -z "$STORAGE_STATE_PATH" ]]; then
  echo "browser login did not return token or storage state" >&2
  cat "$OUT_DIR/browser-login.stdout.json" >&2
  exit 1
fi

github_listener_id="github-rollout-${RUN_ID}"
agentmail_listener_id="agentmail-rollout-${RUN_ID}"
github_client_id="agenthook-github-${RUN_ID}"
agentmail_client_id="agenthook-agentmail-${RUN_ID}"

echo "[3/12] creating AgentHook GitHub listener"
github_listener_body="$OUT_DIR/github-listener-body.json"
write_json "$github_listener_body" \
  "{\"provider\":\"github\",\"listener_id\":\"$github_listener_id\",\"deployment_mode\":\"normal_plan\",\"plain_text_action\":\"store_mysql\",\"use_llm_fallback\":false}"
api_call "create-github-listener" "POST" "$BASE_URL/v1/listeners" "$github_listener_body" \
  -H "Authorization: Bearer $AGENTHOOK_TOKEN" \
  -H 'content-type: application/json' >/dev/null
assert_status "create-github-listener" "201"
GITHUB_WEBHOOK_URL="$(json_get "$OUT_DIR/create-github-listener.json" "webhook_url")"

echo "[4/12] creating AgentHook AgentMail listener"
agentmail_listener_body="$OUT_DIR/agentmail-listener-body.json"
write_json "$agentmail_listener_body" \
  "{\"provider\":\"agentmail\",\"listener_id\":\"$agentmail_listener_id\",\"deployment_mode\":\"normal_plan\",\"plain_text_action\":\"store_mysql\",\"use_llm_fallback\":false}"
api_call "create-agentmail-listener" "POST" "$BASE_URL/v1/listeners" "$agentmail_listener_body" \
  -H "Authorization: Bearer $AGENTHOOK_TOKEN" \
  -H 'content-type: application/json' >/dev/null
assert_status "create-agentmail-listener" "201"
AGENTMAIL_LISTENER_WEBHOOK_URL="$(json_get "$OUT_DIR/create-agentmail-listener.json" "webhook_url")"

if [[ -z "$GITHUB_WEBHOOK_URL" || -z "$AGENTMAIL_LISTENER_WEBHOOK_URL" ]]; then
  echo "listener creation did not return webhook URLs" >&2
  exit 1
fi

echo "[5/12] enumerating owned GitHub repositories"
gh_api --paginate "/user/repos?affiliation=owner&per_page=100" > "$OUT_DIR/github-owned-repos.json"
python3 - "$OUT_DIR/github-owned-repos.json" >"$OUT_DIR/github-owned-repos.flat.json" <<'PY'
import json, sys
repos = json.load(open(sys.argv[1], "r", encoding="utf-8"))
flat = []
for repo in repos:
    if repo.get("archived") or repo.get("disabled"):
        continue
    flat.append({
        "full_name": repo["full_name"],
        "default_branch": repo.get("default_branch"),
        "private": repo.get("private", False),
    })
json.dump(flat, sys.stdout, indent=2)
PY

python3 - "$OUT_DIR/github-owned-repos.flat.json" <<'PY'
import json, sys
repos = json.load(open(sys.argv[1], "r", encoding="utf-8"))
if not repos:
    raise SystemExit("no owned repos found")
PY
touch "$OUT_DIR/github-webhook-results.jsonl"

echo "[6/12] ensuring AgentHook webhook is installed on every owned GitHub repo"
python3 - "$OUT_DIR/github-owned-repos.flat.json" <<'PY' >"$OUT_DIR/github-owned-repos.list"
import json, sys
for repo in json.load(open(sys.argv[1], "r", encoding="utf-8")):
    print(repo["full_name"])
PY

verification_repo=""
while IFS= read -r repo_full_name; do
  hook_file="$OUT_DIR/gh-hooks-$(echo "$repo_full_name" | tr '/' '_').json"
  create_result="created"
  create_reason=""
  if ! gh_api "repos/$repo_full_name/hooks" >"$hook_file" 2>"$hook_file.stderr"; then
    create_result="failed"
    create_reason="$(tr '\n' ' ' <"$hook_file.stderr")"
  else
    existing_hook_url="$(python3 - "$hook_file" "$GITHUB_WEBHOOK_URL" <<'PY'
import json, sys
hooks = json.load(open(sys.argv[1], "r", encoding="utf-8"))
target = sys.argv[2]
for hook in hooks:
    if ((hook.get("config") or {}).get("url")) == target:
        print(hook.get("id", ""))
        break
PY
)"
    if [[ -n "$existing_hook_url" ]]; then
      create_result="already_present"
      create_reason="hook_id=$existing_hook_url"
    else
      payload_file="$OUT_DIR/gh-hook-create-$(echo "$repo_full_name" | tr '/' '_').json"
      write_json "$payload_file" \
        "{\"name\":\"web\",\"active\":true,\"events\":[\"$GITHUB_EVENTS\"],\"config\":{\"url\":$(json_escape "$GITHUB_WEBHOOK_URL"),\"content_type\":\"json\",\"insecure_ssl\":\"0\"}}"
      if gh_api --method POST "repos/$repo_full_name/hooks" --input "$payload_file" >"$payload_file.response" 2>"$payload_file.stderr"; then
        create_reason="hook_created"
      else
        create_result="failed"
        create_reason="$(tr '\n' ' ' <"$payload_file.stderr")"
      fi
    fi
  fi
  if [[ -z "$verification_repo" && "$create_result" != "failed" ]]; then
    verification_repo="$repo_full_name"
  fi
  python3 - "$repo_full_name" "$create_result" "$create_reason" >>"$OUT_DIR/github-webhook-results.jsonl" <<'PY'
import json, sys
print(json.dumps({
    "repo": sys.argv[1],
    "status": sys.argv[2],
    "reason": sys.argv[3],
}))
PY
done <"$OUT_DIR/github-owned-repos.list"

if [[ -z "$verification_repo" ]]; then
  echo "could not find any owned repo that accepted webhook inspection/create" >&2
  exit 1
fi

echo "[7/12] ensuring AgentMail inbox webhook forwards inbound messages to AgentHook"
curl -sS -H "Authorization: Bearer $AGENTMAIL_API_KEY" \
  "https://api.agentmail.to/v0/webhooks" >"$OUT_DIR/agentmail-webhooks.json"
agentmail_existing_id="$(python3 - "$OUT_DIR/agentmail-webhooks.json" "$AGENTMAIL_LISTENER_WEBHOOK_URL" "$EMAIL" <<'PY'
import json, sys
data = json.load(open(sys.argv[1], "r", encoding="utf-8"))
target_url = sys.argv[2]
inbox = sys.argv[3]
for hook in data.get("webhooks", []):
    if hook.get("url") != target_url:
        continue
    if inbox in (hook.get("inbox_ids") or []):
        print(hook.get("webhook_id", ""))
        break
PY
)"
if [[ -z "$agentmail_existing_id" ]]; then
  agentmail_webhook_body="$OUT_DIR/agentmail-webhook-body.json"
  write_json "$agentmail_webhook_body" \
    "{\"url\":$(json_escape "$AGENTMAIL_LISTENER_WEBHOOK_URL"),\"event_types\":[\"$AGENTMAIL_EVENT_TYPES\"],\"inbox_ids\":[$(json_escape "$EMAIL")],\"client_id\":\"$agentmail_client_id\"}"
  curl -sS -X POST "https://api.agentmail.to/v0/webhooks" \
    -H "Authorization: Bearer $AGENTMAIL_API_KEY" \
    -H 'content-type: application/json' \
    --data-binary @"$agentmail_webhook_body" >"$OUT_DIR/agentmail-webhook-create.json"
else
  printf '%s\n' "{\"webhook_id\":\"$agentmail_existing_id\",\"status\":\"already_present\"}" >"$OUT_DIR/agentmail-webhook-create.json"
fi

echo "[8/12] creating a single verification commit in one owned repo"
VERIFY_FILE_PATH=".agenthook-smoke/${RUN_ID}.txt"
VERIFY_CONTENT="$(printf 'agenthook rollout verification %s\n' "$RUN_ID" | base64 | tr -d '\n')"
DEFAULT_BRANCH="$(gh_api "repos/$verification_repo" --jq '.default_branch')"
if gh_api --method PUT "repos/$verification_repo/contents/$VERIFY_FILE_PATH" \
  -f message="AgentHook rollout verification ${RUN_ID}" \
  -f content="$VERIFY_CONTENT" \
  -f branch="$DEFAULT_BRANCH" >"$OUT_DIR/github-verification-commit.json" 2>"$OUT_DIR/github-verification-commit.stderr"; then
  :
else
  echo "verification commit failed for $verification_repo" >&2
  cat "$OUT_DIR/github-verification-commit.stderr" >&2
  exit 1
fi

GITHUB_NEEDLE="$RUN_ID"
poll_listener_events "github" "$github_listener_id" "$GITHUB_NEEDLE" "$OUT_DIR/github-event-match.json"

echo "[9/12] sending a controlled AgentMail message into the shared inbox"
curl -sS -X POST "https://api.agentmail.to/v0/inboxes" \
  -H "Authorization: Bearer $AGENTMAIL_API_KEY" \
  -H 'content-type: application/json' \
  --data-binary "{\"username\":\"agenthook-rollout-$RUN_ID\",\"domain\":\"agentmail.to\",\"client_id\":\"agenthook-rollout-sender-$RUN_ID\"}" \
  >"$OUT_DIR/agentmail-temp-inbox.json"
TEMP_SENDER_INBOX="$(json_get "$OUT_DIR/agentmail-temp-inbox.json" "inbox_id")"
if [[ -z "$TEMP_SENDER_INBOX" ]]; then
  echo "failed to create temporary AgentMail sender inbox" >&2
  cat "$OUT_DIR/agentmail-temp-inbox.json" >&2
  exit 1
fi

agentmail_subject="AgentHook rollout mail ${RUN_ID}"
agentmail_send_body="$OUT_DIR/agentmail-send-body.json"
write_json "$agentmail_send_body" \
  "{\"to\":[$(json_escape "$EMAIL")],\"subject\":$(json_escape "$agentmail_subject"),\"text\":$(json_escape "AgentHook rollout verification message ${RUN_ID}")}"
curl -sS -X POST "https://api.agentmail.to/v0/inboxes/$TEMP_SENDER_INBOX/messages/send" \
  -H "Authorization: Bearer $AGENTMAIL_API_KEY" \
  -H 'content-type: application/json' \
  --data-binary @"$agentmail_send_body" >"$OUT_DIR/agentmail-send.json"

AGENTMAIL_NEEDLE="$agentmail_subject"
poll_listener_events "agentmail" "$agentmail_listener_id" "$AGENTMAIL_NEEDLE" "$OUT_DIR/agentmail-event-match.json"

echo "[10/12] verifying account event feed also contains both flows"
api_call "account-events" "GET" "$BASE_URL/api/events?limit=100" "" \
  -H "Authorization: Bearer $AGENTHOOK_TOKEN" >/dev/null
assert_status "account-events" "200"
python3 - "$OUT_DIR/account-events.json" "$RUN_ID" "$agentmail_subject" <<'PY'
import json, sys
blob = json.dumps(json.load(open(sys.argv[1], "r", encoding="utf-8")))
if sys.argv[2] not in blob:
    raise SystemExit("github rollout marker missing from account events")
if sys.argv[3] not in blob:
    raise SystemExit("agentmail rollout marker missing from account events")
PY

echo "[11/12] verifying Storyboard renders both payloads in the live UI"
run_browser_helper verify-storyboard \
  --out-dir "$OUT_DIR" \
  --storage-state "$STORAGE_STATE_PATH" \
  --github-needle "$GITHUB_NEEDLE" \
  --agentmail-needle "$AGENTMAIL_NEEDLE" \
  --base-url "$BASE_URL" \
  --headless "$BROWSER_HEADLESS" >"$OUT_DIR/storyboard-check.stdout.json"

echo "[12/12] writing rollout summary"
python3 - "$OUT_DIR" "$github_listener_id" "$agentmail_listener_id" "$GITHUB_WEBHOOK_URL" "$AGENTMAIL_LISTENER_WEBHOOK_URL" "$verification_repo" "$RUN_ID" <<'PY' >"$OUT_DIR/summary.json"
import json, os, sys
out_dir, gh_listener, mail_listener, gh_url, mail_url, repo, run_id = sys.argv[1:]
repo_results = []
with open(os.path.join(out_dir, "github-webhook-results.jsonl"), "r", encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if line:
            repo_results.append(json.loads(line))
summary = {
    "run_id": run_id,
    "github_listener_id": gh_listener,
    "agentmail_listener_id": mail_listener,
    "github_webhook_url": gh_url,
    "agentmail_webhook_url": mail_url,
    "verification_repo": repo,
    "storage_state_path": os.path.join(out_dir, "auth-storage.json"),
    "storyboard_screenshot_path": os.path.join(out_dir, "storyboard-verification.png"),
    "repo_results": repo_results,
}
json.dump(summary, sys.stdout, indent=2)
PY

echo "prod rollout completed"
echo "summary=$OUT_DIR/summary.json"
