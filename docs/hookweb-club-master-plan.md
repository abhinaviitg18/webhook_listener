# hookweb.club Master Plan

## 1. Vision
`hookweb.club` is a no-code webhook platform for non-technical users and teams to:
- Register and operate via email and MCP.
- Create deterministic inbound webhook endpoints with typed routes.
- Forward events to destinations (HTTP, MySQL, integrations).
- Add AI-powered memory, summarization, and semantic search.
- Deploy in shared mode or customer-dedicated AWS Lambda mode.

Design principles:
- Security first: tenant isolation, signed requests, secret rotation, least privilege.
- Privacy by design: minimize retained payload data, encryption at rest/in transit, auditable access.
- Determinism first: typed webhook pipelines and strict schema versioning.
- Open core + private integration layer.

---

## 2. URL and Webhook Contract
Inbound webhook URL format:
- `/url/{account}/{type}/{secret}`

Where:
- `account`: tenant/account slug.
- `type`: deterministic handler selector (e.g. `github-push`, `telegram-update`, `generic-json`).
- `secret`: active secret token bound to account + type scope.

Rules:
- One account can have many `type`s.
- One account/type can have any number of active secrets.
- Deleting a secret immediately invalidates matching webhook calls.
- Secret status transitions: `active`, `revoked`, `expired`.

Compatibility support:
- Optional legacy route alias: `/url/{account}/{secret}` maps to `type=generic-json`.

---

## 3. Core Features (MVP -> V2)
### MVP
- Email registration + token auto-generation + token delivery email.
- MCP-based create/list/delete webhook + test webhook APIs.
- Inbound webhook ingestion and validation.
- Forwarding rules engine.
- MySQL persistence for all webhook events and forwarding records.
- Pinecone memory store for processed/filtered summary vectors only.
- Search API Lambda for semantic memory retrieval.
- Standard integration starter: Telegram.
- AWS Lambda shared deployment.

### V1.5
- Customer custom MySQL support (BYO database).
- Event replay controls.
- Dead-letter queue + retry dashboard.
- Per-secret rate limits.

### V2
- Dedicated per-customer Lambda deployment.
- GitHub Actions orchestration to update all customer lambdas.
- Community skill/plugin registry compatibility.
- Private non-open-source integration orchestration service.

---

## 4. High-Level Architecture
### Components
1. API Gateway (public ingress).
2. Ingest Lambda (`hookweb-ingest`) for request validation + normalization.
3. Rules Lambda (`hookweb-rules`) for deterministic routing/forwarding decisions.
4. Worker Lambda (`hookweb-forwarder`) for outbound forwarding/retries.
5. Memory Lambda (`hookweb-memory`) for summarization/embeddings/vector upsert.
6. Search Lambda (`hookweb-search-enterprise`) for enterprise vector query API.
7. Registration Lambda (`hookweb-auth`) for email registration/token lifecycle.
8. MCP API Lambda (`hookweb-mcp`) exposing tooling endpoints.
9. MySQL (RDS Aurora MySQL or MySQL-compatible service).
10. Pinecone (vector DB for summaries/semantic memory).
11. Queue layer (SQS standard + DLQ).
12. Secrets manager (AWS Secrets Manager + KMS).
13. Optional cache (Redis/ElastiCache) for dedupe/idempotency/rate-limits.

### Data split
- MySQL is source of truth for:
  - Accounts, secrets, webhooks, forwarding rules, forwarding logs, retries, audit.
  - Raw inbound payload metadata and retention-controlled payload bodies.
- Pinecone is only for:
  - Filtered webhook entries selected for memory.
  - Generated summaries/meanings and embeddings.

---

## 5. Data Model (MySQL)
### Core tables
- `accounts`
  - `id`, `slug`, `owner_email`, `status`, `plan_tier`, timestamps.
- `account_tokens`
  - `id`, `account_id`, `token_hash`, `scope`, `expires_at`, `revoked_at`.
- `webhook_types`
  - `id`, `account_id`, `type_key`, `schema_json`, `deterministic_handler`, `active`.
- `webhook_secrets`
  - `id`, `account_id`, `type_id`, `secret_hash`, `label`, `status`, `expires_at`.
- `webhook_events`
  - `id`, `account_id`, `type_id`, `secret_id`, `request_id`, `headers_json`, `payload_json`, `payload_sha256`, `received_at`, `processing_status`.
- `forward_rules`
  - `id`, `account_id`, `type_id`, `rule_json`, `priority`, `active`.
- `forward_targets`
  - `id`, `account_id`, `target_type` (`http`, `mysql`, `telegram`, etc), `config_encrypted`.
- `forward_attempts`
  - `id`, `event_id`, `target_id`, `attempt_no`, `status`, `response_code`, `latency_ms`, `error_text`, timestamps.
- `replay_jobs`
  - `id`, `account_id`, `filter_json`, `status`, `started_at`, `ended_at`.
- `audit_logs`
  - `id`, `account_id`, `actor_type`, `actor_id`, `action`, `resource_type`, `resource_id`, `meta_json`, timestamp.
- `customer_deployments`
  - `id`, `account_id`, `mode` (`shared`,`dedicated`), `lambda_name`, `region`, `git_ref`, `status`.
- `external_mysql_connections`
  - `id`, `account_id`, `name`, `host`, `port`, `db_name`, `user`, `password_secret_arn`, `ssl_mode`, `active`.

Indexes/partitioning:
- Partition `webhook_events` by date for retention and cost control.
- Composite indexes: `(account_id, type_id, received_at)`, `(request_id)`, `(processing_status, received_at)`.
- Use generated column for JSON selectors used in rule filters.

---

## 6. Pinecone Memory Model
Index namespace strategy:
- Namespace per account: `acct_{account_id}`.

Metadata fields:
- `event_id`, `account_id`, `type_key`, `summary`, `tags`, `source`, `created_at`, `retention_class`.

Upsert policy:
- Only when rule `store_in_memory=true` or processing outcome matches configured filters.
- Store compact summary + semantic meaning; avoid full sensitive payloads unless explicitly permitted.

Deletion policy:
- On GDPR delete/account purge, remove namespace vectors + MySQL rows according to retention policy.

---

## 7. API Design
### Registration/API auth
- `POST /api/register/email`
  - Input: email.
  - Output: account created/exists response.
  - Action: generate API token, hash/store token, send token by email.

- `POST /api/auth/token/rotate`
- `POST /api/auth/token/revoke`

### Webhook admin
- `POST /api/webhooks/types`
- `GET /api/webhooks/types`
- `POST /api/webhooks/secrets`
- `GET /api/webhooks/secrets`
- `DELETE /api/webhooks/secrets/{secretId}`
- `GET /api/webhooks/url/{type}/{secretId}` (returns `/url/{account}/{type}/{secret}`)

### Event and forwarding
- `POST /url/{account}/{type}/{secret}` (public inbound webhook)
- `GET /api/events`
- `POST /api/events/replay`
- `POST /api/rules`
- `GET /api/rules`
- `POST /api/forward-targets`

### Memory/search
- `POST /api/memory/search` (shared)
- `POST /enterprise/search` (enterprise Lambda)
  - Inputs: query, filters (`type`, `date_range`, `tags`, `confidence_threshold`), top_k.
  - Returns vector matches + linked event metadata.

### MCP endpoints
- `mcp.webhook.create`
- `mcp.webhook.secret.create`
- `mcp.webhook.secret.delete`
- `mcp.webhook.list`
- `mcp.webhook.test`
- `mcp.forwarding.rule.create`
- `mcp.deploy.customer.lambda`

---

## 8. AI Provider Strategy (Groq, Cerebras, OpenRouter)
Provider abstraction:
- Unified provider adapter interface: `summarize(payload)`, `embed(text)`.
- Config per account with fallback chain:
  1. Account-configured primary (Groq/Cerebras/OpenRouter).
  2. Platform default fallback.

Token/security handling:
- Store provider tokens encrypted in AWS Secrets Manager.
- Never log plaintext tokens.
- Rotate via API + audit trail.

Resilience:
- Circuit breaker per provider.
- Retry with exponential backoff + jitter.
- Fallback on provider outage/timeouts.

---

## 9. Forwarding Rules and Integrations
Rules engine shape:
- Deterministic JSON rules with explicit priority and stop/continue semantics.

Example rule fields:
- Matchers: `type`, `json_path` equals/contains/regex, header conditions.
- Actions:
  - `forward_http`
  - `forward_mysql`
  - `forward_telegram`
  - `store_in_memory`
  - `drop`

Standard integrations (launch):
- Telegram (Bot API).
- Generic HTTP target.
- MySQL target.

Non-standard integrations:
- Plugin contract via private integration skill/service.
- Community skills can register adapters through approved manifest + sandboxed execution runtime.

---

## 10. Skill Strategy
### Open-source core skill
Name: `hookweb-core-ops`
Purpose:
- Setup account, webhook types/secrets, rules, memory filters, and deployments via APIs.

### Private integration skill (not open-source)
Name: `hookweb-integration-ops-private`
Purpose:
- Enterprise integrations, internal adapter generation, privileged deployment actions.

Compatibility rule:
- Community skills remain supported by public adapter interface and MCP capability checks.

### Skill import/export repository design (Claude/Cursor/Codex compatibility)
Goal:
- Import skills from external repositories and normalize them into hookweb skill records.
- Export hookweb skills back into repository formats for Claude Code, Cursor, and Codex.

Canonical skill package (`hookweb-skill.json`):
- `skill_key`, `type_key`, `skill_prompt`, `match_contains`, `forced_action`, `memory_write_mode`, `priority`, `enabled`.
- Optional `metadata` (`source_platform`, `source_repo`, `version`, `hash`).

Import flow:
1. Download skill manifest from remote repo (zip/git/raw).
2. Validate schema and sanitize fields (length limits, forbidden actions).
3. Convert platform-specific syntax to canonical package.
4. Write to `webhook_skills` with source metadata.
5. Run deterministic dry-run fixtures before enabling.

Export flow:
1. Read canonical skill records from `webhook_skills`.
2. Convert to target platform template:
- Claude Code skill markdown package.
- Cursor prompt/rule package.
- Codex `SKILL.md` package.
3. Emit downloadable artifact (`zip`) and signed manifest.

Security controls:
- Do not execute imported code during import.
- Allow only approved action names (`store_mysql`, `forward_http`, `forward_telegram`, `no_action`).
- Require tenant-scoped auth and audit logging for import/export.

Future APIs:
- `POST /api/skills/import`
- `POST /api/skills/export`
- `GET /api/skills/repository/templates`

---

## 10.1 Auto-promote deterministic pipeline
Lifecycle:
- `validated` -> `shadow` -> `active`.

Cutoff parameters:
- `AUTOPROMOTE_MIN_CONFIDENCE` (default `0.88`): minimum LLM confidence to count toward validation.
- `AUTOPROMOTE_VALIDATED_TO_SHADOW` (default `2`): number of high-confidence candidate hits before shadow.
- `AUTOPROMOTE_SHADOW_TO_ACTIVE` (default `3`): deterministic shadow samples required before active.
- `AUTOPROMOTE_MIN_SUCCESS_RATE` (default `0.90`): minimum shadow success ratio for activation.

Rules:
1. New type candidate discovered by resolver (Groq/Cerebras) creates disabled signature + pending transform.
2. When validation cutoff is met, signature is enabled and transform status moves to `shadow`.
3. While in shadow, deterministic matches are measured.
4. If shadow volume and success-rate cutoffs pass, transform is promoted to `active`.
5. If shadow success is below cutoff, candidate rolls back to `validated` and signature is disabled.

AI Recruiter override:
- `DETERMINISTIC_ONLY_TYPE_KEYS` defaults to `ai-recruiter-inbox-message`.
- Types in this list skip LLM fallback in ingest/action flow and stay deterministic-only.

---

## 11. Deployment Plan (AWS Lambda)
### Shared stack
- Multi-tenant Lambdas + account isolation in data and auth layers.

### Dedicated customer stack
- Separate Lambda (or set of Lambdas) per customer.
- Dedicated environment variables, secrets, and optional VPC.
- Deployment entry in `customer_deployments`.

### GitHub Actions rollout model
Workflows:
1. `deploy-shared.yml`: deploy shared services.
2. `deploy-customer-lambda.yml`: deploy one customer.
3. `deploy-all-customers.yml`: matrix job over customer deployment manifest.

Manifest source:
- `infra/customers/customers.json` with account->lambda mapping.

Safety:
- Concurrency control per customer.
- Canary alias shift (10% -> 50% -> 100%).
- Auto rollback on CloudWatch alarm breach.

---

## 12. Security, Privacy, Compliance
Security controls:
- HMAC signature support + secret path token validation.
- WAF + rate limiting + IP reputation controls.
- Idempotency key support to prevent replay side effects.
- Per-account and per-secret quotas.
- Encrypt data at rest (RDS, Pinecone metadata minimalism, Secrets Manager).

Privacy controls:
- Field-level redaction before persistence.
- Configurable payload retention windows.
- Right-to-delete job for account data in MySQL + Pinecone.
- Audit logs for admin and token operations.

Compliance readiness:
- SOC2 control mapping (access, logging, change mgmt).
- GDPR/DPDP style data subject erasure workflow.

---

## 13. Scalability and Reliability
Scalability:
- Queue decoupling between ingest and processing.
- Horizontal Lambda concurrency limits with reserved concurrency.
- Batch forwarding worker and adaptive retries.

Reliability:
- DLQ for failed events.
- Replay from durable MySQL records.
- Outbound circuit breakers per target integration.

Observability:
- Structured logs with account/type/request correlation IDs.
- Metrics: ingest QPS, p95 latency, failure ratios, retry counts, memory upsert lag.
- Tracing: API Gateway -> Lambda -> SQS -> worker chain.

---

## 14. Suggested Additional Features
- Visual no-code rule builder with templates.
- Event payload schema inference and drift alerts.
- Test payload simulator per webhook type.
- Secret expiry reminders and auto-rotation.
- "Safe mode" for new rules (dry-run with shadow logs).
- Per-integration health dashboards.
- Human-readable AI summaries in inbox/email digests.
- Data residency policy selection (region pinning).
- Team roles/RBAC and approval workflows.

---

## 15. Use Cases
1. Non-technical founder receives Stripe-style events and forwards only payment success to Telegram.
2. Operations manager registers via email, receives token, and configures webhook entirely via MCP.
3. Support team stores only summarized webhook meaning in Pinecone for semantic lookup.
4. Enterprise customer runs dedicated Lambda with isolated secrets and custom MySQL.
5. Developer rotates one compromised secret without affecting other active secrets.
6. Community integration skill publishes adapter for niche CRM destination.
7. Customer queries memory search API to find "failed invoice events last week".

---

## 16. Edge Cases
1. Secret deleted during in-flight retries.
2. Duplicate webhook delivery from sender.
3. Invalid `type` value but valid account/secret.
4. Payload too large for Lambda direct processing.
5. Outbound integration timeout storms.
6. Pinecone outage while MySQL remains healthy.
7. Email registration flood/abuse attempts.
8. Customer custom MySQL SSL misconfiguration.
9. Race between token rotate and API requests using old token.
10. Dedicated customer lambda drift from shared base version.

---

## 17. Test Plan
### Unit tests
- Secret lifecycle: create/revoke/delete/expire.
- URL parser and type routing.
- Rule evaluation determinism (priority/order/stop behavior).
- Provider abstraction fallbacks (Groq -> Cerebras -> OpenRouter).

### Integration tests
- Register by email and verify token delivery trigger.
- End-to-end ingest -> rules -> forward -> MySQL persistence.
- Pinecone upsert only on memory-qualified events.
- Search API returns account-scoped filtered results.
- Custom MySQL connection and forwarding writes.

### E2E browser tests (no-code user)
- Signup flow, token retrieval notice UI, create webhook, send test event, confirm forwarding logs.
- Telegram integration setup and message receipt check.

### Security tests
- Fuzz malformed webhook URLs.
- Replay and idempotency tests.
- Rate-limit and abuse simulations.
- Secret brute force throttling.

### Performance tests
- Burst ingest test (e.g. 5k RPS).
- P95 end-to-end latency under load.
- Queue drain and retry behavior under downstream failures.

### Chaos tests
- Disable Pinecone during processing.
- Inject provider timeout failures.
- Simulate RDS failover and validate recovery.

---

## 18. Phased Delivery Plan
### Phase 0 (Week 1)
- Finalize contracts, data model, infra IaC baseline.
- Build registration + token email flow.

### Phase 1 (Weeks 2-4)
- Ingest API, secret/type routing, MySQL persistence.
- Basic rules engine + HTTP forwarding.

### Phase 2 (Weeks 5-6)
- Memory pipeline (summary + Pinecone) + search API Lambda.
- Telegram integration + MCP operations.

### Phase 3 (Weeks 7-8)
- Dedicated customer lambda deployment model.
- GitHub Actions mass customer rollout.

### Phase 4 (Weeks 9-10)
- Hardening: security, reliability, DR, compliance controls.
- Public open-core packaging and docs.

---

## 19. Open Source Boundary
Open-source:
- Core webhook engine.
- Rules DSL and standard integrations.
- Community skill SDK and adapter contracts.

Closed/private:
- Proprietary integration skill orchestration.
- Enterprise deployment automations and privileged adapters.

---

## 20. Acceptance Criteria
1. Non-technical user can register by email and receive a usable token.
2. User can create type-aware webhook URL in `/url/{account}/{type}/{secret}` format.
3. Multiple secrets per type are supported; deleted secret invalidates endpoint.
4. All webhook events and forwarding logs persist in MySQL.
5. Only eligible processed summaries are stored in Pinecone.
6. Search API Lambda can query Pinecone with account-scoped filters.
7. Shared and dedicated Lambda deployment modes both function.
8. GitHub Actions can roll out updates to all customer lambdas safely.
9. MCP operations support create/fetch/deploy workflows.
