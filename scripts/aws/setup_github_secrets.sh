#!/usr/bin/env bash
set -euo pipefail

if ! command -v gh >/dev/null 2>&1; then
  echo "gh cli is required"
  exit 1
fi

REPO="${REPO:-abhinaviitg18/webhook_listener}"
ENV_NAME="${ENV_NAME:-production}"

AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-$(awk -F= '/^AWS_ACCESS_KEY_ID=/{print $2;exit}' .env 2>/dev/null || true)}"
AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-$(awk -F= '/^AWS_SECRET_ACCESS_KEY=/{print $2;exit}' .env 2>/dev/null || true)}"
AWS_REGION="${AWS_REGION:-$(awk -F= '/^AWS_REGION=/{print $2;exit}' .env 2>/dev/null || true)}"
EC2_INSTANCE_ID="${EC2_INSTANCE_ID:-}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/hookweb/prod/env}"

if [[ -z "$AWS_ACCESS_KEY_ID" || -z "$AWS_SECRET_ACCESS_KEY" || -z "$AWS_REGION" ]]; then
  echo "missing AWS credentials (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_REGION)"
  exit 1
fi
if [[ -z "$EC2_INSTANCE_ID" ]]; then
  echo "set EC2_INSTANCE_ID before running this script"
  exit 1
fi

echo "setting GitHub environment secrets on $REPO ($ENV_NAME)"
printf '%s' "$AWS_ACCESS_KEY_ID" | gh secret set AWS_ACCESS_KEY_ID --repo "$REPO" --env "$ENV_NAME" --body -
printf '%s' "$AWS_SECRET_ACCESS_KEY" | gh secret set AWS_SECRET_ACCESS_KEY --repo "$REPO" --env "$ENV_NAME" --body -
printf '%s' "$AWS_REGION" | gh secret set AWS_REGION --repo "$REPO" --env "$ENV_NAME" --body -
printf '%s' "$EC2_INSTANCE_ID" | gh secret set EC2_INSTANCE_ID --repo "$REPO" --env "$ENV_NAME" --body -
printf '%s' "$APP_ENV_SSM_PARAM" | gh secret set APP_ENV_SSM_PARAM --repo "$REPO" --env "$ENV_NAME" --body -

echo "done"
