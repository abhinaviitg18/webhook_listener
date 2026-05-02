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
- `MAIL_OUTBOUND_PROVIDER`
- `MAIL_AGENTHOOK_BASE_URL`
- `MAIL_AGENTHOOK_ORIGIN_SECRET`
- `MAIL_INTERNAL_SHARED_SECRET`

Inbound receiving remains AWS SES based.

Outbound sending is provider-selectable:

- `ses`
- `resend`
- `postmark`
- `smtp`
- `zeptomail`

Provider env vars:

- SES
  - `MAIL_AWS_REGION`
- Resend
  - `MAIL_RESEND_API_KEY`
  - `MAIL_RESEND_BASE_URL`
- Postmark
  - `MAIL_POSTMARK_SERVER_TOKEN`
  - `MAIL_POSTMARK_BASE_URL`
- SMTP
  - `MAIL_SMTP_HOST`
  - `MAIL_SMTP_PORT`
  - `MAIL_SMTP_USERNAME`
  - `MAIL_SMTP_PASSWORD`
  - `MAIL_SMTP_USE_TLS`
- ZeptoMail
  - `MAIL_ZEPTOMAIL_API_KEY`
  - `MAIL_ZEPTOMAIL_BASE_URL`

If outbound provider config is missing, the mail API still boots for mailbox listing and inbound processing, but send/reply returns `mail sender not configured`.

## Send and reply API smoke test

List mailboxes:

```bash
curl -sS \
  -H "Authorization: Bearer $AGENTHOOK_TOKEN" \
  http://127.0.0.1:8080/v1/mailboxes
```

Send a new message:

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $AGENTHOOK_TOKEN" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:8080/v1/mailboxes/$MAILBOX_ID/send \
  -d '{
    "to":["you@example.com"],
    "subject":"AgentHook send test",
    "text_body":"Plain text body",
    "html_body":"<p>Plain text body</p>"
  }'
```

Reply to an existing message:

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $AGENTHOOK_TOKEN" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:8080/v1/messages/$MESSAGE_ID/reply \
  -d '{
    "text_body":"Replying from AgentHook"
  }'
```

Recommended operator smoke-test order:

1. Verify `GET /v1/mailboxes` returns the expected mailbox.
2. Send one outbound test email and confirm HTTP `201`.
3. Confirm the outbound row appears in `GET /v1/mailboxes/{mailbox_id}/messages`.
4. Confirm the recipient actually receives the email.
5. Send a real inbound email into the mailbox address.
6. Confirm SES -> S3 -> mail ingress -> AgentHook still works.
7. Reply to that inbound message and verify threading in the recipient mailbox.

Outside AgentHook, operators still need:

- a verified sending domain on the chosen provider
- SPF/DKIM configured for that provider
- mailbox domain alignment that matches the visible `From` address

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

Reusable provisioning guidance:

- `scripts/aws/setup_ses_mail_domain.sh`
- `docs/setup-ses-mail-domain.md`
