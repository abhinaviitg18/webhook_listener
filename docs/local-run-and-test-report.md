# hookweb.club Local Build and Test Report

## What was built
- Go API platform with typed webhook ingestion endpoint: `/url/{account}/{type}/{secret}`.
- Auto-type ingestion endpoint: `/url/{account}/{secret}`.
- API auth/tokenized registration (`/api/register/email`) with account-scoped token issuance.
- Webhook type management with plain text action policy and LLM fallback.
- Secret lifecycle support with secret revoke invalidation behavior.
- MCP-style processing flow:
  1. Message arrives.
  2. Pinecone query executes first to fetch relevant context.
  3. Decide action by type plain text rule, else LLM suggestion.
  4. Execute action through function registry (`store_mysql`, `forward_http`, `forward_telegram`, etc).
- Telegram integration included.
- Two-stage type resolver:
  - deterministic fingerprint/signature scoring first.
  - Groq + Cerebras LLM fallback for low-confidence payloads.
- Deterministic transform registry with `dsl` and sandboxed `wasm` execution paths.
- Master prompt + per-type skill engine:
  - Configure decision strategy per account.
  - Attach multiple skills per webhook type.
  - Drive action selection + memory write behavior (`update_or_insert`, `insert_only`, `none`).
- TiDB-compatible MySQL schema migration included.
- Sample website at `/` describing the platform (UI intentionally minimal).

## Message type behavior
For each `type_key`, behavior is:
1. If `plain_text_action` is set:
- Deterministically run that action.
- Example: `telegram-update -> forward_telegram`.

2. If `plain_text_action` is empty and `use_llm_fallback=true`:
- Call LLM with payload + Pinecone context + available functions.
- Execute selected action.

3. If neither condition gives an action:
- Default action is `store_mysql`.

## APIs implemented
- `POST /api/register/email`
- `POST /api/webhooks/types`
- `GET /api/webhooks/types`
- `POST /api/webhooks/secrets`
- `DELETE /api/webhooks/secrets/{secretID}`
- `POST /api/forward-targets`
- `GET /api/events`
- `POST /url/{account}/{type}/{secret}`
- `POST /url/{account}/{secret}` (auto-detect type)
- `POST /api/resolver/signatures`
- `GET /api/resolver/signatures`
- `POST /api/resolver/transforms`
- `GET /api/resolver/transforms`
- `POST /api/resolver/classify`
- `POST /api/resolver/transform?type_key=<type>`
- `POST /api/policy/master`
- `GET /api/policy/master`
- `POST /api/policy/skills`
- `GET /api/policy/skills?type_key=<type>`
- `GET /healthz`
- `GET /` (sample website)
- `GET /app` (listener-centric product webpage)
- `POST /v1/listeners`
- `GET /v1/listeners`
- `POST /v1/listeners/{listener_id}/secrets`
- `GET /v1/listeners/{listener_id}/events`
- `POST /ingest/{account}/{provider}/{webhook_id}/{secret}`

## Automated tests run
Command:
```bash
go test -mod=mod ./...
```

Coverage highlights:
- `internal/service/processor_test.go`
  - Plain text action overrides LLM suggestions.
  - LLM fallback executes when plain text action is absent.
  - Skill-based forced action (`no_action`) and memory skip behavior.
  - Skill-based `insert_only` memory behavior.
  - LLM param-driven memory override (`params.memory_write_mode`).
  - Deterministic-only type bypasses LLM fallback even when webhook type enables it.
- `internal/service/resolver_test.go`
  - Deterministic classification across multiple webhook structures (GitHub, Stripe, Slack).
  - LLM disagreement resolves to best-confidence type and continues processing.
  - DSL transform extraction validation.
  - Auto-promote transition test: `validated -> shadow -> active` with cutoffs.
- `internal/transport/http/handlers_test.go`
  - Local end-to-end flow:
    registration -> type create -> secret create -> webhook post -> event list.
- `internal/transport/http/auto_route_test.go`
  - Auto route classification + deterministic transform + processed event assertion.
- `internal/transport/http/policy_suite_test.go`
  - End-to-end API flow for policy + skill endpoints and webhook decision outcomes.
  - Verifies Pinecone write suppression (`none`) and insert-only mode.

## Manual tests run
Server launch:
```bash
PORT=8081 USE_IN_MEMORY_STORE=true go run -mod=mod ./cmd/server
```

Manual API script:
```bash
BASE_URL=http://localhost:8081 EMAIL=7204909316@agentmail.to ./scripts/manual_api_test.sh
```

Auto-type and multi-structure script:
```bash
BASE_URL=http://localhost:<port> EMAIL=7204909316@agentmail.to ./scripts/manual_auto_type_test.sh
```

Policy/skill test script (6 payload variants):
```bash
BASE_URL=http://localhost:<port> EMAIL=7204909316@agentmail.to ./scripts/manual_policy_skill_test.sh
```

Observed results:
- Registration succeeded and token returned.
- Type creation succeeded.
- Secret creation succeeded and URL generated in required format.
- Webhook accepted and processed.
- Event persistence verified via `/api/events`.
- Telegram forwarding failed as expected without `TELEGRAM_BOT_TOKEN`; failure state persisted in event logs.
- Multi-structure auto route handled deterministic signatures and produced canonical payloads:
  - GitHub push -> `{\"repo\":\"org/repo\",\"commit\":\"abc123\"}`
  - Stripe payment -> `{\"event\":\"payment_intent.succeeded\",\"amount\":3500}`
  - Slack event -> `{\"team\":\"T123\",\"event_type\":\"message\"}`
- Unknown payload path fell back to LLM full-JSON processing after secret validation (no manual-review stop).
- Policy skill behavior was validated with six webhook payload families (github, stripe, slack, telegram, heartbeat, incident):
  - heartbeat -> `no_action` and `memory_write_mode=none`
  - incident -> `store_mysql` with `memory_write_mode=update_or_insert`
- Deterministic-only AI Recruiter flow (`type_key=ai-recruiter-inbox-message`) bypassed resolver classification and returned resolution source `deterministic_locked`.

CLI utility:
```bash
echo '{\"repository\":{\"full_name\":\"org/repo\"},\"head_commit\":{\"id\":\"abc\"}}' | ./scripts/hookweb_cli.sh classify --account <account_id>
echo '{\"repository\":{\"full_name\":\"org/repo\"},\"head_commit\":{\"id\":\"abc\"}}' | ./scripts/hookweb_cli.sh transform --account <account_id> --type github-push
```

## Browser test run
Command:
```bash
npx -y playwright@1.52.0 screenshot --device='Desktop Chrome' http://localhost:8081/ /tmp/hookweb-home.png
```

Result:
- Sample website loaded successfully in a real browser engine and screenshot captured.
- Updated regression screenshot after auto-promote changes: `/tmp/hookweb-home-autopromote.png`.
- Mobile screenshot run with Playwright iPhone profile could not complete because WebKit runtime was not installed in this environment.

## TiDB / ScaleKit / Pinecone setup status
- TiDB: local bootstrap script added (`scripts/start_tidb_local.sh`).
- Pinecone: API connectivity script added (`scripts/check_pinecone.sh`).
- ScaleKit: OIDC/config validation script added (`scripts/check_scalekit.sh`).

## Live Pinecone setup (April 8, 2026)
- Configured a fresh Pinecone API key in project `.env`.
- Created index `hookweb-club` in serverless AWS `us-east-1` with dimension `8` and metric `cosine`.
- Updated `PINECONE_INDEX_URL` to the created index host and namespace `hookweb`.
- Verified index readiness and query response (`matches: []` on empty namespace).

Validation commands run:
```bash
PINECONE_API_KEY=<from .env> ./scripts/check_pinecone.sh
go test -mod=mod ./...
```

End-to-end API flow (with Pinecone configured) run on local server:
- register -> create type -> create secret -> post webhook -> list events.
- Result: processed event with action `store_mysql`.

Browser smoke check:
```bash
npx -y playwright@1.52.0 screenshot --device='Desktop Chrome' http://localhost:<port>/ /tmp/hookweb-home-after-pinecone.png
```
- Result: homepage loaded and screenshot captured successfully.

Listener v1 flow snapshots (April 14, 2026):
- API JSON snapshots directory: `/tmp/hookweb-v1-snapshots`
- API screenshot snapshots:
  - `/tmp/hookweb-api-ingest-v1.png`
  - `/tmp/hookweb-api-events-v1.png`
- Website screenshots:
  - `/tmp/hookweb-home-v1.png`
  - `/tmp/hookweb-app-v1.png`

ScaleKit + grouped secret dashboard update (April 14, 2026):
- Added ScaleKit bearer auth fallback in middleware (OIDC userinfo path).
- Added listener event payload field `secret_id` and grouped-by-secret dashboard UI on `/app`.
- New snapshot set:
  - JSON: `/tmp/hookweb-v1-snapshots-2`
  - UI grouped view: `/tmp/hookweb-app-grouped-secrets.png`
  - UI empty state: `/tmp/hookweb-app-blank.png`
  - API screenshot (second secret ingest): `/tmp/hookweb-api-second-secret-ingest.png`
  - API screenshot (list events with secret ids): `/tmp/hookweb-api-list-events-grouped.png`

Provider account creation note:
- Automated account creation for Pinecone/ScaleKit/TiDB cloud using only email is provider-controlled and usually requires captcha/manual verification in browser.
- This codebase is prepared to consume fresh credentials created with `7204909316@agentmail.to` and avoids hard-coding provider endpoints.

## Production readiness design choices already applied
- Connection pool settings for MySQL/TiDB.
- Account-scoped token and secret hashing.
- Request size cap for webhook payload ingestion.
- Structured action registry for deterministic function execution.
- Safe fallback behavior when external LLM/Pinecone not configured.

## Next production-hardening steps
1. Replace in-memory store with TiDB in runtime (`USE_IN_MEMORY_STORE=false`, set `TIDB_DSN`).
2. Add asynchronous queue (SQS/Kafka) between ingest and action execution.
3. Add HMAC signature validation and idempotency table.
4. Implement retries + DLQ for external targets (Telegram/HTTP).
5. Add OpenTelemetry traces and metrics.
