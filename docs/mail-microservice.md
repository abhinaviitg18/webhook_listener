# AgentHook Mail Microservice

AgentHook mail extends the short webhook identity so every active secret can also receive email:

- HTTP: `https://app.agenthook.store/{public_alias}.{secret}`
- Email: `{public_alias}.{secret}@app.agenthook.store`

The mail stack is intentionally separate from the main AgentHook app:

1. Amazon SES receives inbound mail for `app.agenthook.store`
2. SES stores the raw MIME object in S3
3. S3 triggers the `agenthook-mail-ingress` Lambda
4. The mail Lambda parses the mailbox local part as `{public_alias}.{secret}`
5. It resolves the active AgentHook account alias plus webhook secret
6. It stores normalized mailbox, thread, message, attachment, and delivery metadata in the mail schema
7. It forwards the normalized payload into the normal AgentHook processor through the canonical short URL

## Local development

The repo now exposes a separate mail API server:

```bash
go run ./cmd/mailapi
```

The mail API reuses the normal AgentHook account tokens and serves:

- `GET /v1/mailboxes`
- `GET /v1/mailboxes/{mailbox_id}/messages`
- `GET /v1/messages/{message_id}`
- `POST /v1/mailboxes/{mailbox_id}/send`
- `POST /v1/messages/{message_id}/reply`
- `POST /internal/ingress/s3-event`

## Environment

Mail-specific variables:

- `MAIL_DB_DSN`
- `MAIL_DOMAIN`
- `MAIL_AWS_REGION`
- `MAIL_INBOUND_BUCKET`
- `MAIL_AGENTHOOK_BASE_URL`
- `MAIL_AGENTHOOK_ORIGIN_SECRET`
- `MAIL_INTERNAL_SHARED_SECRET`

## DNS and Cloudflare

Required records for inbound SES receiving on `app.agenthook.store`:

- `app.agenthook.store MX 10 inbound-smtp.us-east-1.amazonaws.com`
- SES verification TXT record for `app.agenthook.store`
- SES Easy DKIM CNAME records for `app.agenthook.store`

Recommended outbound alignment:

- `bounce.app.agenthook.store MX 10 feedback-smtp.us-east-1.amazonses.com`
- `bounce.app.agenthook.store TXT "v=spf1 include:amazonses.com ~all"`
- `_dmarc.app.agenthook.store TXT "v=DMARC1; p=none; rua=mailto:dmarc@agenthook.store; adkim=s; aspf=s"`

Cloudflare guidance:

- Keep the existing proxied web record for `app.agenthook.store`
- Publish SES DKIM CNAME records as DNS-only
- MX/TXT records remain DNS-only
- If Cloudflare Email Routing is enabled on `app.agenthook.store`, remove those routing MX/TXT records before enabling SES inbound receiving
