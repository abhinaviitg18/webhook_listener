#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  MAIL_DOMAIN=app.agenthook.store \
  AWS_REGION=us-east-1 \
  S3_BUCKET_NAME=mail-app-agenthook-store-inbound \
  LAMBDA_FUNCTION_NAME=agenthook-mail-ingress \
  MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME=agenthook-mail-receipt-guard \
  scripts/aws/setup_ses_mail_domain.sh

Environment variables:
  MAIL_DOMAIN              Required. Receiving domain, for example app.agenthook.store
  AWS_REGION               Optional. Defaults to us-east-1
  S3_BUCKET_NAME           Optional. Defaults to mail-<domain-slug>-inbound
  MAIL_FROM_SUBDOMAIN      Optional. Defaults to bounce.<MAIL_DOMAIN>
  RECEIPT_RULE_SET         Optional. Defaults to mail-ingress-<domain-slug>
  RECEIPT_RULE_NAME        Optional. Defaults to store-raw-mail-<domain-slug>
  LAMBDA_FUNCTION_NAME     Optional. Used only to print S3 notification wiring commands
  MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME Optional. When provided and present, SES invokes it synchronously before S3 delivery.
  RAW_PREFIX               Optional. Defaults to raw/
  ATTACHMENT_PREFIX        Optional. Defaults to attachments/
  RAW_RETENTION_DAYS       Optional. Defaults to 30
  EVENTBRIDGE_ENABLED      Optional. true/false. Defaults to false
  CONFIG_SET_NAME          Optional. Defaults to mail-events-<domain-slug> when EventBridge is enabled
  ENABLE_S3_LAMBDA_TRIGGER Optional. true/false. Defaults to false

Notes:
  - This script automates AWS only.
  - DNS changes are printed for manual entry in Cloudflare or any other DNS provider.
  - One inbound bucket per domain is the intended default.
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need aws
need jq

MAIL_DOMAIN="${MAIL_DOMAIN:-}"
AWS_REGION="${AWS_REGION:-us-east-1}"
RAW_PREFIX="${RAW_PREFIX:-raw/}"
ATTACHMENT_PREFIX="${ATTACHMENT_PREFIX:-attachments/}"
RAW_RETENTION_DAYS="${RAW_RETENTION_DAYS:-30}"
EVENTBRIDGE_ENABLED="${EVENTBRIDGE_ENABLED:-false}"
ENABLE_S3_LAMBDA_TRIGGER="${ENABLE_S3_LAMBDA_TRIGGER:-false}"
LAMBDA_FUNCTION_NAME="${LAMBDA_FUNCTION_NAME:-}"
MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME="${MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME:-}"

if [[ -z "$MAIL_DOMAIN" ]]; then
  echo "MAIL_DOMAIN is required" >&2
  exit 1
fi

MAIL_DOMAIN="$(printf '%s' "$MAIL_DOMAIN" | tr '[:upper:]' '[:lower:]' | xargs)"
DOMAIN_SLUG="$(printf '%s' "$MAIL_DOMAIN" | tr '.' '-' | tr -cd 'a-z0-9-')"
S3_BUCKET_NAME="${S3_BUCKET_NAME:-mail-${DOMAIN_SLUG}-inbound}"
MAIL_FROM_SUBDOMAIN="${MAIL_FROM_SUBDOMAIN:-bounce.${MAIL_DOMAIN}}"
RECEIPT_RULE_SET="${RECEIPT_RULE_SET:-mail-ingress-${DOMAIN_SLUG}}"
RECEIPT_RULE_NAME="${RECEIPT_RULE_NAME:-store-raw-mail-${DOMAIN_SLUG}}"
CONFIG_SET_NAME="${CONFIG_SET_NAME:-mail-events-${DOMAIN_SLUG}}"

WORK_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

bool_is_true() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|y|on) return 0 ;;
    *) return 1 ;;
  esac
}

aws_json() {
  aws "$@" --region "$AWS_REGION" --output json
}

account_id() {
  aws sts get-caller-identity --region "$AWS_REGION" --query 'Account' --output text
}

ACCOUNT_ID="$(account_id)"
RECEIPT_RULE_ARN="arn:aws:ses:${AWS_REGION}:${ACCOUNT_ID}:receipt-rule-set/${RECEIPT_RULE_SET}:receipt-rule/${RECEIPT_RULE_NAME}"
EVENT_BUS_ARN="arn:aws:events:${AWS_REGION}:${ACCOUNT_ID}:event-bus/default"
TRIGGER_STATUS="not-requested"

ensure_bucket() {
  if aws s3api head-bucket --bucket "$S3_BUCKET_NAME" >/dev/null 2>&1; then
    echo "reusing existing bucket: ${S3_BUCKET_NAME}"
  else
    echo "creating bucket: ${S3_BUCKET_NAME}"
    if [[ "$AWS_REGION" == "us-east-1" ]]; then
      aws s3api create-bucket --bucket "$S3_BUCKET_NAME" --region "$AWS_REGION" >/dev/null
    else
      aws s3api create-bucket \
        --bucket "$S3_BUCKET_NAME" \
        --region "$AWS_REGION" \
        --create-bucket-configuration "LocationConstraint=${AWS_REGION}" >/dev/null
    fi
  fi

  aws s3api put-public-access-block \
    --bucket "$S3_BUCKET_NAME" \
    --public-access-block-configuration \
    BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true >/dev/null

  aws s3api put-bucket-ownership-controls \
    --bucket "$S3_BUCKET_NAME" \
    --ownership-controls 'Rules=[{ObjectOwnership=BucketOwnerEnforced}]' >/dev/null

  aws s3api put-bucket-encryption \
    --bucket "$S3_BUCKET_NAME" \
    --server-side-encryption-configuration \
    '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}' >/dev/null

  cat >"$WORK_DIR/lifecycle.json" <<EOF
{
  "Rules": [
    {
      "ID": "expire-raw-mail",
      "Status": "Enabled",
      "Filter": {
        "Prefix": "${RAW_PREFIX}"
      },
      "Expiration": {
        "Days": ${RAW_RETENTION_DAYS}
      }
    },
    {
      "ID": "expire-attachments",
      "Status": "Enabled",
      "Filter": {
        "Prefix": "${ATTACHMENT_PREFIX}"
      },
      "Expiration": {
        "Days": ${RAW_RETENTION_DAYS}
      }
    }
  ]
}
EOF
  aws s3api put-bucket-lifecycle-configuration \
    --bucket "$S3_BUCKET_NAME" \
    --lifecycle-configuration "file://$WORK_DIR/lifecycle.json" >/dev/null

  cat >"$WORK_DIR/bucket-policy.json" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowSESPutsFor${DOMAIN_SLUG}",
      "Effect": "Allow",
      "Principal": {
        "Service": "ses.amazonaws.com"
      },
      "Action": "s3:PutObject",
      "Resource": "arn:aws:s3:::${S3_BUCKET_NAME}/${RAW_PREFIX}*",
      "Condition": {
        "StringEquals": {
          "AWS:SourceAccount": "${ACCOUNT_ID}"
        },
        "ArnLike": {
          "AWS:SourceArn": "${RECEIPT_RULE_ARN}"
        }
      }
    }
  ]
}
EOF
  aws s3api put-bucket-policy \
    --bucket "$S3_BUCKET_NAME" \
    --policy "file://$WORK_DIR/bucket-policy.json" >/dev/null
}

ensure_domain_verification() {
  DOMAIN_VERIFICATION_TOKEN="$(aws ses verify-domain-identity \
    --domain "$MAIL_DOMAIN" \
    --region "$AWS_REGION" \
    --query 'VerificationToken' \
    --output text)"
}

ensure_dkim_tokens() {
  aws ses verify-domain-dkim \
    --domain "$MAIL_DOMAIN" \
    --region "$AWS_REGION" \
    --query 'DkimTokens[]' \
    --output text | tr '\t' '\n' >"$WORK_DIR/dkim_tokens.txt"
}

ensure_mail_from() {
  aws ses set-identity-mail-from-domain \
    --identity "$MAIL_DOMAIN" \
    --mail-from-domain "$MAIL_FROM_SUBDOMAIN" \
    --behavior-on-mx-failure UseDefaultValue \
    --region "$AWS_REGION" >/dev/null
}

ensure_receipt_rule_set() {
  if aws ses describe-receipt-rule-set --rule-set-name "$RECEIPT_RULE_SET" --region "$AWS_REGION" >/dev/null 2>&1; then
    echo "reusing existing receipt rule set: ${RECEIPT_RULE_SET}"
  else
    echo "creating receipt rule set: ${RECEIPT_RULE_SET}"
    aws ses create-receipt-rule-set \
      --rule-set-name "$RECEIPT_RULE_SET" \
      --region "$AWS_REGION" >/dev/null
  fi
  aws ses set-active-receipt-rule-set \
    --rule-set-name "$RECEIPT_RULE_SET" \
    --region "$AWS_REGION" >/dev/null
}

ensure_receipt_guard_permission() {
  if [[ -z "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" ]]; then
    return 0
  fi
  if ! aws lambda get-function \
    --region "$AWS_REGION" \
    --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" >/dev/null 2>&1; then
    return 0
  fi
  if ! aws lambda get-policy \
    --region "$AWS_REGION" \
    --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" \
    --query 'Policy' \
    --output text 2>/dev/null | grep -q "AllowSESMailReceiptGuard${DOMAIN_SLUG}"; then
    aws lambda add-permission \
      --region "$AWS_REGION" \
      --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" \
      --statement-id "AllowSESMailReceiptGuard${DOMAIN_SLUG}" \
      --action lambda:InvokeFunction \
      --principal ses.amazonaws.com \
      --source-account "$ACCOUNT_ID" \
      --source-arn "$RECEIPT_RULE_ARN" >/dev/null
  fi
}

ensure_receipt_rule() {
  local actions_json lambda_arn
  actions_json="$(jq -cn --arg bucket "$S3_BUCKET_NAME" --arg prefix "$RAW_PREFIX" '
    [
      {
        S3Action: {
          BucketName: $bucket,
          ObjectKeyPrefix: $prefix
        }
      }
    ]')"

  if [[ -n "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" ]]; then
    if lambda_arn="$(aws lambda get-function \
      --region "$AWS_REGION" \
      --function-name "$MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME" \
      --query 'Configuration.FunctionArn' \
      --output text 2>/dev/null)"; then
      actions_json="$(jq -cn --arg guard_arn "$lambda_arn" --arg bucket "$S3_BUCKET_NAME" --arg prefix "$RAW_PREFIX" '
        [
          {
            LambdaAction: {
              FunctionArn: $guard_arn,
              InvocationType: "RequestResponse"
            }
          },
          {
            S3Action: {
              BucketName: $bucket,
              ObjectKeyPrefix: $prefix
            }
          }
        ]')"
    else
      echo "mail receipt guard lambda not found, continuing with S3-only receipt rule: ${MAIL_RECEIPT_GUARD_LAMBDA_FUNCTION_NAME}"
    fi
  fi

  cat >"$WORK_DIR/receipt-rule.json" <<EOF
{
  "Name": "${RECEIPT_RULE_NAME}",
  "Enabled": true,
  "TlsPolicy": "Optional",
  "Recipients": ["${MAIL_DOMAIN}"],
  "Actions": ${actions_json},
  "ScanEnabled": true
}
EOF

  if aws ses describe-receipt-rule \
    --rule-set-name "$RECEIPT_RULE_SET" \
    --rule-name "$RECEIPT_RULE_NAME" \
    --region "$AWS_REGION" >/dev/null 2>&1; then
    echo "updating existing receipt rule: ${RECEIPT_RULE_NAME}"
    aws ses update-receipt-rule \
      --rule-set-name "$RECEIPT_RULE_SET" \
      --rule "file://$WORK_DIR/receipt-rule.json" \
      --region "$AWS_REGION" >/dev/null
  else
    echo "creating receipt rule: ${RECEIPT_RULE_NAME}"
    aws ses create-receipt-rule \
      --rule-set-name "$RECEIPT_RULE_SET" \
      --rule "file://$WORK_DIR/receipt-rule.json" \
      --region "$AWS_REGION" >/dev/null
  fi
}

ensure_eventbridge_destination() {
  if ! bool_is_true "$EVENTBRIDGE_ENABLED"; then
    return 0
  fi

  if aws sesv2 get-configuration-set \
    --configuration-set-name "$CONFIG_SET_NAME" \
    --region "$AWS_REGION" >/dev/null 2>&1; then
    echo "reusing existing configuration set: ${CONFIG_SET_NAME}"
  else
    echo "creating configuration set: ${CONFIG_SET_NAME}"
    aws sesv2 create-configuration-set \
      --configuration-set-name "$CONFIG_SET_NAME" \
      --region "$AWS_REGION" >/dev/null
  fi

  cat >"$WORK_DIR/eventbridge-destination.json" <<EOF
{
  "Enabled": true,
  "MatchingEventTypes": [
    "SEND",
    "REJECT",
    "BOUNCE",
    "COMPLAINT",
    "DELIVERY",
    "OPEN",
    "CLICK"
  ],
  "EventBridgeDestination": {
    "EventBusArn": "${EVENT_BUS_ARN}"
  }
}
EOF

  if aws sesv2 get-configuration-set-event-destinations \
    --configuration-set-name "$CONFIG_SET_NAME" \
    --region "$AWS_REGION" \
    --output json | jq -e '.EventDestinations[]? | select(.EventDestinationName=="eventbridge-default")' >/dev/null; then
    echo "updating EventBridge destination on configuration set: ${CONFIG_SET_NAME}"
    aws sesv2 update-configuration-set-event-destination \
      --configuration-set-name "$CONFIG_SET_NAME" \
      --event-destination-name eventbridge-default \
      --event-destination "file://$WORK_DIR/eventbridge-destination.json" \
      --region "$AWS_REGION" >/dev/null
  else
    echo "creating EventBridge destination on configuration set: ${CONFIG_SET_NAME}"
    aws sesv2 create-configuration-set-event-destination \
      --configuration-set-name "$CONFIG_SET_NAME" \
      --event-destination-name eventbridge-default \
      --event-destination "file://$WORK_DIR/eventbridge-destination.json" \
      --region "$AWS_REGION" >/dev/null
  fi
}

ensure_lambda_trigger() {
  if ! bool_is_true "$ENABLE_S3_LAMBDA_TRIGGER"; then
    TRIGGER_STATUS="disabled"
    return 0
  fi
  if [[ -z "$LAMBDA_FUNCTION_NAME" ]]; then
    TRIGGER_STATUS="skipped_missing_lambda_name"
    return 0
  fi

  local lambda_arn trigger_id statement_id notification_json
  if ! lambda_arn="$(aws lambda get-function \
    --region "$AWS_REGION" \
    --function-name "$LAMBDA_FUNCTION_NAME" \
    --query 'Configuration.FunctionArn' \
    --output text 2>/dev/null)"; then
    echo "warning: Lambda function ${LAMBDA_FUNCTION_NAME} was not found; skipping S3 trigger wiring" >&2
    TRIGGER_STATUS="skipped_missing_lambda_function"
    return 0
  fi

  trigger_id="ses-mail-${DOMAIN_SLUG}"
  statement_id="allow-s3-${DOMAIN_SLUG}"

  if ! aws lambda get-policy \
    --region "$AWS_REGION" \
    --function-name "$LAMBDA_FUNCTION_NAME" \
    --output json 2>/dev/null | jq -e --arg sid "$statement_id" '.Policy | fromjson | .Statement[]? | select(.Sid == $sid)' >/dev/null; then
    aws lambda add-permission \
      --region "$AWS_REGION" \
      --function-name "$LAMBDA_FUNCTION_NAME" \
      --statement-id "$statement_id" \
      --action lambda:InvokeFunction \
      --principal s3.amazonaws.com \
      --source-arn "arn:aws:s3:::${S3_BUCKET_NAME}" \
      --source-account "$ACCOUNT_ID" >/dev/null
  fi

  notification_json="$(aws s3api get-bucket-notification-configuration \
    --bucket "$S3_BUCKET_NAME" \
    --region "$AWS_REGION" \
    --output json)"
  if [[ -z "$notification_json" ]]; then
    notification_json='{}'
  fi

  printf '%s' "$notification_json" | jq \
    --arg id "$trigger_id" \
    --arg arn "$lambda_arn" \
    --arg prefix "$RAW_PREFIX" \
    '
      .LambdaFunctionConfigurations = (
        ((.LambdaFunctionConfigurations // []) | map(select(.Id != $id))) + [
          {
            Id: $id,
            LambdaFunctionArn: $arn,
            Events: ["s3:ObjectCreated:*"],
            Filter: {
              Key: {
                FilterRules: [
                  {Name: "prefix", Value: $prefix}
                ]
              }
            }
          }
        ]
      )
    ' >"$WORK_DIR/bucket-notifications.json"

  jq -nc \
    --arg bucket "$S3_BUCKET_NAME" \
    --slurpfile cfg "$WORK_DIR/bucket-notifications.json" \
    '{Bucket: $bucket, NotificationConfiguration: $cfg[0]}' >"$WORK_DIR/put-bucket-notifications.json"

  aws s3api put-bucket-notification-configuration \
    --region "$AWS_REGION" \
    --cli-input-json "file://$WORK_DIR/put-bucket-notifications.json" >/dev/null

  TRIGGER_STATUS="configured"
}

print_summary() {
  cat <<EOF

== AWS resources
mail_domain=${MAIL_DOMAIN}
aws_region=${AWS_REGION}
s3_bucket_name=${S3_BUCKET_NAME}
raw_prefix=${RAW_PREFIX}
attachment_prefix=${ATTACHMENT_PREFIX}
mail_from_subdomain=${MAIL_FROM_SUBDOMAIN}
receipt_rule_set=${RECEIPT_RULE_SET}
receipt_rule_name=${RECEIPT_RULE_NAME}
receipt_rule_arn=${RECEIPT_RULE_ARN}
lambda_function_name=${LAMBDA_FUNCTION_NAME:-not-provided}
eventbridge_enabled=${EVENTBRIDGE_ENABLED}
config_set_name=$(bool_is_true "$EVENTBRIDGE_ENABLED" && printf '%s' "$CONFIG_SET_NAME" || printf 'not-enabled')
s3_lambda_trigger_status=${TRIGGER_STATUS}
EOF
}

print_dns_block() {
  echo
  echo "== DNS records to add"
  echo "provider_hint=Cloudflare"
  echo "note=Keep web A/CNAME records as-is. Set all SES DKIM CNAMEs to DNS only."
  printf 'TYPE\tNAME\tVALUE\tPRIORITY\tPROXY\n'
  printf 'TXT\t%s\t%s\t-\tDNS only\n' "_amazonses.${MAIL_DOMAIN}" "${DOMAIN_VERIFICATION_TOKEN}"
  while IFS= read -r token; do
    printf 'CNAME\t%s\t%s\t-\tDNS only\n' "${token}._domainkey.${MAIL_DOMAIN}" "${token}.dkim.amazonses.com"
  done <"$WORK_DIR/dkim_tokens.txt"
  printf 'MX\t%s\t%s\t10\tDNS only\n' "${MAIL_DOMAIN}" "inbound-smtp.${AWS_REGION}.amazonaws.com"
  printf 'MX\t%s\t%s\t10\tDNS only\n' "${MAIL_FROM_SUBDOMAIN}" "feedback-smtp.${AWS_REGION}.amazonses.com"
  printf 'TXT\t%s\t%s\t-\tDNS only\n' "${MAIL_FROM_SUBDOMAIN}" "v=spf1 include:amazonses.com ~all"
  printf 'TXT\t%s\t%s\t-\tDNS only\n' "_dmarc.${MAIL_DOMAIN}" "v=DMARC1; p=none; adkim=s; aspf=s"
}

print_validation() {
  cat <<EOF

== Validation commands
aws ses get-identity-verification-attributes --region ${AWS_REGION} --identities ${MAIL_DOMAIN}
aws ses get-identity-dkim-attributes --region ${AWS_REGION} --identities ${MAIL_DOMAIN}
aws ses get-identity-mail-from-domain-attributes --region ${AWS_REGION} --identities ${MAIL_DOMAIN}
aws ses describe-active-receipt-rule-set --region ${AWS_REGION}
aws s3api get-bucket-policy --bucket ${S3_BUCKET_NAME}
dig +short MX ${MAIL_DOMAIN}
dig +short TXT _amazonses.${MAIL_DOMAIN}
dig +short TXT _dmarc.${MAIL_DOMAIN}

== Next step: S3 notification wiring
MAIL_DOMAIN must match the mail service environment variable MAIL_DOMAIN.
MAIL_INBOUND_BUCKET must be set to ${S3_BUCKET_NAME} on the mail ingress deployment.
Wire S3 ObjectCreated events on prefix ${RAW_PREFIX} to the Lambda that parses inbound mail.
EOF

  if [[ -n "$LAMBDA_FUNCTION_NAME" ]]; then
    cat <<EOF

Suggested Lambda lookup:
aws lambda get-function --region ${AWS_REGION} --function-name ${LAMBDA_FUNCTION_NAME} --query 'Configuration.FunctionArn' --output text

Suggested S3 notification payload:
{
  "LambdaFunctionConfigurations": [
    {
      "LambdaFunctionArn": "arn:aws:lambda:${AWS_REGION}:${ACCOUNT_ID}:function:${LAMBDA_FUNCTION_NAME}",
      "Events": ["s3:ObjectCreated:*"],
      "Filter": {
        "Key": {
          "FilterRules": [
            { "Name": "prefix", "Value": "${RAW_PREFIX}" }
          ]
        }
      }
    }
  ]
}
EOF
  fi
}

ensure_bucket
ensure_domain_verification
ensure_dkim_tokens
ensure_mail_from
ensure_receipt_rule_set
ensure_receipt_guard_permission
ensure_receipt_rule
ensure_eventbridge_destination
ensure_lambda_trigger
print_summary
print_dns_block
print_validation
