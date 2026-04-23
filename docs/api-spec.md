# agenthook.store API Spec (Current Build)

## Auth model
- Registration returns account-scoped bearer token.
- Admin endpoints require `Authorization: Bearer <token>`.
- Auth supports two bearer token modes:
  - Local agenthook token (`/api/register/email` output).
  - ScaleKit bearer token (verified via OIDC discovery + `userinfo`), with auto-provisioned local account by email slug.

## 1) Register
`POST /api/register/email`

Request:
```json
{"email":"7204909316@agentmail.to"}
```

Response (201):
```json
{
  "account": {"id":"...","slug":"7204909316","owner_email":"7204909316@agentmail.to","created_at":"..."},
  "token": "...",
  "note": "send this token by email in production"
}
```

## Listener-first v1 API (new)

### Create listener
`POST /v1/listeners`

Request:
```json
{
  "provider": "agentmail",
  "listener_id": "wh_agentmail_001",
  "deployment_mode": "normal_plan",
  "plain_text_action": "store_mysql",
  "use_llm_fallback": false
}
```

Response includes provider-specific secret URL:
```json
{
  "listener_id":"wh_agentmail_001",
  "provider":"agentmail",
  "deployment_mode":"multitenant",
  "webhook_url":"/ingest/{account}/agentmail/wh_agentmail_001/{secret}"
}
```

Deployment mode normalization:
- `normal_plan` -> `multitenant`
- `enterprise` or `single_tenant` -> `single_tenant`

### List listeners
`GET /v1/listeners`

### Create another secret for existing listener
`POST /v1/listeners/{listener_id}/secrets`
```json
{"provider":"agentmail"}
```

### List events for one listener
`GET /v1/listeners/{listener_id}/events?provider=agentmail&limit=20`

Event rows include both storage forms:
- `raw_payload_json`
- `payload_json` (canonical/processed JSON)
- `processed_text`
- `secret_id` (used by dashboard grouping)

### Provider ingress route
`POST /ingest/{account}/{provider}/{webhook_id}/{secret}`

Examples:
- `/ingest/acme/agentmail/wh_001/sec_abc`
- `/ingest/acme/slack/wh_002/sec_xyz`
- `/ingest/acme/jira/wh_003/sec_pqr`

## 2) Create webhook type
`POST /api/webhooks/types`

Request:
```json
{
  "type_key": "telegram-update",
  "plain_text_action": "forward_telegram",
  "use_llm_fallback": true
}
```

Behavior:
- If `plain_text_action` is set, processing is deterministic.
- If unset and `use_llm_fallback=true`, LLM picks action.

## 3) List webhook types
`GET /api/webhooks/types`

## 4) Create secret
`POST /api/webhooks/secrets`

Request:
```json
{"type_key":"telegram-update"}
```

Response:
```json
{
  "secret": {"id":"...","status":"active"},
  "secret_value": "...",
  "webhook_url": "/url/7204909316/telegram-update/<secret>"
}
```

## 5) Delete secret
`DELETE /api/webhooks/secrets/{secretID}`

Behavior:
- Revoked secret makes that specific webhook path invalid.

## 6) Create forward target
`POST /api/forward-targets`

Telegram example:
```json
{"target_type":"telegram","config":{"chat_id":"123456"}}
```

HTTP example:
```json
{"target_type":"http","config":{"url":"https://example.com/webhook"}}
```

## 7) Receive webhook (public)
`POST /url/{account}/{type}/{secret}`

Example:
`POST /url/7204909316/telegram-update/abc123`

Pipeline:
1. Validate account/type/secret.
2. If `VERIFY_HTC_SIGNATURE=true`, verify `X-HTC-Webhook-Timestamp` and `X-HTC-Webhook-Signature`.
3. Persist event in MySQL/TiDB.
4. Query Pinecone context.
5. Decide action (type rule first, else LLM).
6. Execute function (MCP-style action).
7. Persist final status.

## 7b) Receive webhook with auto type detection
`POST /url/{account}/{secret}`

Pipeline:
1. Resolve account + active secret.
2. If `VERIFY_HTC_SIGNATURE=true`, verify `X-HTC-Webhook-Timestamp` and `X-HTC-Webhook-Signature`.
3. If secret belongs to deterministic-only type (`DETERMINISTIC_ONLY_TYPE_KEYS`), bypass classifier and process directly.
4. Otherwise deterministic signature match (`webhook_type_signatures`).
5. If low confidence, Groq + Cerebras dual classification.
6. If agreement/disagreement resolved: candidate signature/transform may be auto-promoted (`validated -> shadow -> active`) by configured cutoffs.
7. If still unknown or provider error: continue with LLM action on full JSON (secret already validated).
8. AI Recruiter deterministic-only types skip this fallback path.
9. Apply active deterministic transform (`dsl` or `wasm`) only when type is confidently resolved.
10. Execute action engine (MCP-style) on canonical payload or full JSON fallback.
11. Idempotency check by source event id (`event_id`/provider ids) avoids duplicate re-processing.

## 8) List events
`GET /api/events?limit=50`

Returns recent account-scoped webhook events and action statuses.

## 9) Service endpoints
- `GET /healthz`
- `GET /` (sample website)

## 10) Resolver/Transform admin endpoints
- `POST /api/resolver/signatures`
- `GET /api/resolver/signatures`
- `POST /api/resolver/transforms`
- `GET /api/resolver/transforms?type_key=<type>`
- `POST /api/resolver/classify` (dry-run classification)
- `POST /api/resolver/transform?type_key=<type>` (dry-run deterministic transform)

## 11) Policy + Skill endpoints
- `POST /api/policy/master`
- `GET /api/policy/master`
- `POST /api/policy/skills`
- `GET /api/policy/skills?type_key=<type>`

Master prompt request:
```json
{
  "prompt_text":"Use skills first, then LLM fallback.",
  "updated_by":"ops@agenthook.store"
}
```

Skill request:
```json
{
  "type_key":"generic-json",
  "skill_key":"drop-heartbeat",
  "skill_prompt":"Ignore heartbeat metrics payloads",
  "match_contains":"heartbeat,metrics",
  "forced_action":"no_action",
  "memory_write_mode":"none",
  "priority":1,
  "enabled":true
}
```

Memory write modes:
- `update_or_insert` (default): query Pinecone and update best matching record when score/type match, otherwise create new.
- `insert_only`: force creation of a fresh Pinecone memory record.
- `none`: skip Pinecone write for this webhook.

## 12) Auto-promote env parameters
- `AUTOPROMOTE_ENABLED=true`
- `AUTOPROMOTE_MIN_CONFIDENCE=0.88`
- `AUTOPROMOTE_VALIDATED_TO_SHADOW=2`
- `AUTOPROMOTE_SHADOW_TO_ACTIVE=3`
- `AUTOPROMOTE_MIN_SUCCESS_RATE=0.90`
- `DETERMINISTIC_ONLY_TYPE_KEYS=ai-recruiter-inbox-message`
