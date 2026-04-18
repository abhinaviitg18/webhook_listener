---
name: hookweb-core-ops
description: Manage hookweb.club webhook operations through APIs and MCP, including account bootstrap, token lifecycle, typed webhook URL setup, secret management, deterministic forwarding rules, event fetch/replay, and AWS Lambda deployment flows. Use this skill when users ask to set up webhooks for non-technical teams, automate webhook management, troubleshoot webhook delivery, or deploy shared and customer-dedicated lambda stacks.
---

# Hookweb Core Ops

## Overview

Use this skill to operate `hookweb.club` end to end through API-first workflows. Focus on deterministic webhook behavior with URL format `/url/{account}/{type}/{secret}`, secure secret handling, MySQL-first persistence, and optional Pinecone-backed memory search for processed summaries.

## Core Workflow

1. Bootstrap account and auth
2. Configure webhook type and secrets
3. Create forwarding rules and targets
4. Validate ingest and fetch events
5. Enable memory storage/search for eligible events
6. Deploy shared or dedicated customer lambdas

## Step 1: Bootstrap Account and Token

1. Register user email with `POST /api/register/email`.
2. Confirm token generation and email dispatch completed.
3. Store only token hash in DB; never persist plaintext token.
4. Expose token rotation/revoke endpoints for operational hygiene.

Operational checks:
- Enforce per-IP registration throttle.
- Include idempotency key for repeated registration requests.
- Write audit log entries for token lifecycle actions.

## Step 2: Configure Type-Aware Webhooks and Secrets

1. Create type definition with `POST /api/webhooks/types`.
2. Create one or more secrets with `POST /api/webhooks/secrets`.
3. Return canonical webhook URL in format `/url/{account}/{type}/{secret}`.
4. On secret deletion, enforce immediate invalidation and reject future calls.

Validation rules:
- `account`, `type`, and `secret` are required path segments.
- `type` maps to deterministic handler contract.
- Permit multiple active secrets per account/type.

## Step 3: Configure Forwarding Targets and Rules

1. Create target(s) such as `http`, `mysql`, `telegram`.
2. Add deterministic rules with explicit priority and stop/continue behavior.
3. Persist all inbound and forwarding outcomes in MySQL.
4. Support customer-provided MySQL connections for forwarding storage.

Rule guardrails:
- Validate JSONPath selectors before save.
- Reject overlapping ambiguous rules unless priority resolves order.
- Capture versioned rule snapshots for rollback.

## Step 4: Validate Ingest, Fetch, and Replay

1. Send controlled test payloads to inbound webhook URL.
2. Verify event persistence in MySQL and forwarding attempt logs.
3. Use `GET /api/events` for diagnostics and `POST /api/events/replay` for recovery.
4. Confirm idempotency and duplicate handling logic.

## Step 5: Configure Memory and Search

1. Enable memory rules only for selected filtered/processed webhook events.
2. Generate summary/meaning from approved AI provider adapter.
3. Store vector memory in Pinecone with account namespace.
4. Use search API lambda (`/enterprise/search`) for scoped semantic retrieval.

Provider strategy:
- Prefer configured provider: Groq or Cerebras or OpenRouter.
- Fallback to platform default on failures.
- Never log provider tokens.

## Step 6: Deploy Shared or Dedicated Customer Lambdas

1. Shared mode deploy updates common multi-tenant stack.
2. Dedicated mode deploys separate lambda per customer.
3. Keep deployment metadata (`lambda_name`, `region`, `version`) in MySQL.
4. Trigger GitHub Actions workflow to roll updates across all customer lambdas.

Release safeguards:
- Canary alias shift and automatic rollback on alarms.
- Concurrency limits for mass customer rollout.
- Drift detection against customer deployment manifest.

## MCP Operations Mapping

- `mcp.webhook.create`: create type and return canonical URL.
- `mcp.webhook.secret.create`: add secret and show activation status.
- `mcp.webhook.secret.delete`: revoke secret and invalidate endpoint.
- `mcp.webhook.list`: enumerate types/secrets/rules.
- `mcp.webhook.test`: fire test event and return trace IDs.
- `mcp.webhook.fetch`: fetch latest events with filters.
- `mcp.deploy.customer.lambda`: deploy dedicated customer lambda.

## Security and Privacy Checklist

- Require token auth for all admin APIs and scoped RBAC checks.
- Apply per-account and per-secret rate limits.
- Enforce request size limits and schema validation.
- Encrypt secrets with KMS-backed Secrets Manager.
- Redact sensitive fields before log/storage per policy.
- Keep audit trail for account, secret, rule, token, and deployment actions.
- Use MySQL for webhook and forwarding source-of-truth records.
- Use Pinecone only for filtered processed summaries/meaning.

## Output Expectations

When using this skill, produce:
1. Exact API calls executed or required payload contracts.
2. Created webhook URLs and secret status outcomes.
3. Verification notes for MySQL and forwarding states.
4. Deployment summary for shared and/or dedicated lambda updates.
5. Any blockers with least-risk fallback options.
