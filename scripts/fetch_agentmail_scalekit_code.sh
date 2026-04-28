#!/usr/bin/env bash
set -euo pipefail

INBOX_EMAIL="${1:-7204909316@agentmail.to}"
MAX_AGE_MIN="${MAX_AGE_MIN:-30}"

AGENTMAIL_API_KEY="${AGENTMAIL_API_KEY:-}"
if [[ -z "$AGENTMAIL_API_KEY" && -f "local.env" ]]; then
  AGENTMAIL_API_KEY=$(awk -F= '/^AGENTMAIL_API_KEY=/{sub(/^AGENTMAIL_API_KEY=/,"");print;exit}' local.env)
fi
if [[ -z "$AGENTMAIL_API_KEY" && -f ".env" ]]; then
  AGENTMAIL_API_KEY=$(awk -F= '/^AGENTMAIL_API_KEY=/{sub(/^AGENTMAIL_API_KEY=/,"");print;exit}' .env)
fi
if [[ -z "$AGENTMAIL_API_KEY" ]]; then
  echo "AGENTMAIL_API_KEY missing"
  exit 1
fi

RAW_JSON=$(mktemp)
curl -sS -H "Authorization: Bearer $AGENTMAIL_API_KEY" \
  "https://api.agentmail.to/v0/inboxes/$INBOX_EMAIL/messages" >"$RAW_JSON"

python3 - "$RAW_JSON" "$MAX_AGE_MIN" <<'PY'
import json, re, sys
from datetime import datetime, timezone, timedelta

path = sys.argv[1]
max_age_min = int(sys.argv[2])
raw = open(path, "r", errors="ignore").read()
data = json.loads(raw, strict=False)
messages = data.get("messages", [])

otp_re = re.compile(r"(?:verification code is|otp|code)\D*(\d{6})", re.I)
now = datetime.now(timezone.utc)
cutoff = now - timedelta(minutes=max_age_min)

best = None
for m in messages:
    frm = m.get("from", [])
    frm_s = ", ".join(frm) if isinstance(frm, list) else str(frm or "")
    subject = str(m.get("subject", ""))
    if "scalekit" not in (frm_s + " " + subject).lower():
        continue
    ts_raw = str(m.get("timestamp", ""))
    if ts_raw.endswith("Z"):
        ts_raw = ts_raw[:-1] + "+00:00"
    try:
        ts = datetime.fromisoformat(ts_raw)
    except Exception:
        continue
    if ts < cutoff:
        continue
    hay = subject
    match = otp_re.search(hay)
    if not match:
        continue
    code = match.group(1)
    if best is None or ts > best["ts"]:
        best = {"ts": ts, "code": code, "subject": subject, "from": frm_s}

if best is None:
    print("no_recent_scalekit_code_found")
else:
    print(f"scalekit_code={best['code']}")
    print(f"timestamp={best['ts'].isoformat()}")
    print(f"from={best['from']}")
    print(f"subject={best['subject']}")
PY
