# Reusable SES Mail Domain Setup

Use this guide and the companion script to provision a mail domain for:

- `app.agenthook.store`
- AI Recruiter mail domains
- any future single-tenant customer domain

The goal is to keep the setup reusable and isolated:

- one inbound S3 bucket per domain
- one SES receipt rule set per domain
- one mail identity per domain
- separate raw MIME storage for AgentHook vs AI Recruiter

## What the setup script creates

Run:

```bash
MAIL_DOMAIN=app.agenthook.store \
AWS_REGION=us-east-1 \
S3_BUCKET_NAME=mail-app-agenthook-store-inbound \
MAIL_FROM_SUBDOMAIN=bounce.app.agenthook.store \
RECEIPT_RULE_SET=mail-ingress-app-agenthook-store \
RECEIPT_RULE_NAME=store-raw-mail-app-agenthook-store \
LAMBDA_FUNCTION_NAME=agenthook-mail-ingress \
bash scripts/aws/setup_ses_mail_domain.sh
```

The script will:

1. create or reuse the inbound S3 bucket
2. enable bucket encryption, public access blocking, and lifecycle rules
3. apply an SES-only bucket policy scoped to the receipt rule
4. create or refresh SES domain verification
5. create or refresh SES Easy DKIM tokens
6. configure the custom MAIL FROM subdomain
7. create or update the SES receipt rule set and receipt rule
8. optionally create an SES EventBridge-backed configuration set
9. print the DNS records to add manually

The script is idempotent and safe to rerun.

## Required inputs

- `MAIL_DOMAIN`
  - receiving domain, for example `app.agenthook.store`
- `AWS_REGION`
  - default `us-east-1`
- `S3_BUCKET_NAME`
  - recommended default: `mail-<domain-slug>-inbound`
- `MAIL_FROM_SUBDOMAIN`
  - recommended default: `bounce.<MAIL_DOMAIN>`
- `RECEIPT_RULE_SET`
  - recommended default: `mail-ingress-<domain-slug>`
- `RECEIPT_RULE_NAME`
  - recommended default: `store-raw-mail-<domain-slug>`
- `LAMBDA_FUNCTION_NAME`
  - optional, used to print S3 notification wiring commands

Optional inputs:

- `RAW_PREFIX`
- `ATTACHMENT_PREFIX`
- `RAW_RETENTION_DAYS`
- `EVENTBRIDGE_ENABLED`
- `CONFIG_SET_NAME`

## Naming convention

Recommended domain slug examples:

- `app.agenthook.store` -> `app-agenthook-store`
- `mail.airecruiter.io` -> `mail-airecruiter-io`
- `customer.example.com` -> `customer-example-com`

Recommended resources:

- bucket: `mail-<domain-slug>-inbound`
- bounce subdomain: `bounce.<mail-domain>`
- receipt rule set: `mail-ingress-<domain-slug>`
- receipt rule: `store-raw-mail-<domain-slug>`

## Bucket isolation policy

Use one inbound bucket per domain by default.

Examples:

- AgentHook:
  - `mail-app-agenthook-store-inbound`
- AI Recruiter:
  - `mail-mail-airecruiter-io-inbound`
- future single-tenant customer:
  - `mail-customer-example-com-inbound`

This prevents raw MIME from different products or customers from landing in the same bucket.

## Cloudflare and DNS changes

The script prints the exact records to add. For Cloudflare:

- keep existing proxied web records unchanged
- set SES DKIM `CNAME` records to `DNS only`
- MX and TXT records are always `DNS only`
- if Cloudflare Email Routing is enabled on the same hostname, remove those routing records before enabling SES inbound receiving

Expected records per domain:

- inbound MX
- verification TXT
- three DKIM CNAMEs
- MAIL FROM MX
- MAIL FROM SPF TXT
- DMARC TXT

## Wiring S3 to the mail ingress Lambda

The script creates SES -> S3 delivery only.
S3 notification wiring remains an explicit operator step so existing bucket notifications are not overwritten unexpectedly.

You should:

1. confirm the mail ingress Lambda exists
2. grant S3 invoke permission to that Lambda if needed
3. configure bucket notifications on the raw mail prefix

The target Lambda environment must match the domain:

- `MAIL_DOMAIN=<receiving-domain>`
- `MAIL_INBOUND_BUCKET=<bucket-name>`

For example, AgentHook should point to:

- `MAIL_DOMAIN=app.agenthook.store`
- `MAIL_INBOUND_BUCKET=mail-app-agenthook-store-inbound`

AI Recruiter should point to its own domain and bucket values.

## Validating the setup

After publishing DNS, run:

```bash
aws ses get-identity-verification-attributes --region us-east-1 --identities app.agenthook.store
aws ses get-identity-dkim-attributes --region us-east-1 --identities app.agenthook.store
aws ses get-identity-mail-from-domain-attributes --region us-east-1 --identities app.agenthook.store
aws ses describe-active-receipt-rule-set --region us-east-1
aws s3api get-bucket-policy --bucket mail-app-agenthook-store-inbound
dig +short MX app.agenthook.store
dig +short TXT _amazonses.app.agenthook.store
dig +short TXT _dmarc.app.agenthook.store
```

You want to see:

- domain verification succeeds
- DKIM status succeeds
- MAIL FROM status succeeds
- MX points to `inbound-smtp.<region>.amazonaws.com`

## End-to-end test checklist

1. send a message to `{alias}.{secret}@<domain>`
2. confirm the raw MIME lands in the correct per-domain bucket
3. confirm S3 triggers the mail ingress Lambda
4. confirm the message is normalized and forwarded into AgentHook
5. confirm replies send through SES using the same domain identity

## Future single-tenant use

To onboard a new single-tenant customer domain:

1. choose a new domain-specific bucket
2. run the same setup script with the new `MAIL_DOMAIN`
3. add the printed DNS records in the customer’s DNS provider
4. deploy the mail ingress environment with the matching `MAIL_DOMAIN` and `MAIL_INBOUND_BUCKET`
5. test with a single mailbox address before opening the full domain
