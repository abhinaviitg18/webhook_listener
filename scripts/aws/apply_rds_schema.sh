#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
DB_NAME="${DB_NAME:-agenthook}"
MIGRATION_FILE="${MIGRATION_FILE:-db/migrations/001_init.sql}"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1"; exit 1; }
}
need mysql

resolve_from_secret() {
  local arn="$1"
  command -v aws >/dev/null 2>&1 || { echo "aws cli required for RDS secret lookup"; exit 1; }
  local secret_json
  secret_json=$(aws secretsmanager get-secret-value --region "$AWS_REGION" \
    --secret-id "$arn" --query SecretString --output text)
  export RDS_HOST="${RDS_HOST:-$(echo "$secret_json" | jq -r '.host // empty')}"
  export RDS_PORT="${RDS_PORT:-$(echo "$secret_json" | jq -r '.port // 3306')}"
  export RDS_USER="${RDS_USER:-$(echo "$secret_json" | jq -r '.username // empty')}"
  export RDS_PASSWORD="${RDS_PASSWORD:-$(echo "$secret_json" | jq -r '.password // empty')}"
  export DB_NAME="${DB_NAME:-$(echo "$secret_json" | jq -r '.dbname // "agenthook"')}"
}

if [[ -n "${RDS_SECRET_ARN:-}" ]]; then
  command -v jq >/dev/null 2>&1 || { echo "jq required when using RDS_SECRET_ARN"; exit 1; }
  resolve_from_secret "$RDS_SECRET_ARN"
fi

if [[ -z "${RDS_HOST:-}" || -z "${RDS_USER:-}" || -z "${RDS_PASSWORD:-}" ]]; then
  echo "set RDS_HOST, RDS_USER, RDS_PASSWORD (or RDS_SECRET_ARN)"
  exit 1
fi
RDS_PORT="${RDS_PORT:-3306}"

if [[ ! -f "$MIGRATION_FILE" ]]; then
  echo "migration file not found: $MIGRATION_FILE"
  exit 1
fi

echo "==> creating database $DB_NAME on $RDS_HOST:$RDS_PORT"
mysql -h "$RDS_HOST" -P "$RDS_PORT" -u "$RDS_USER" "-p$RDS_PASSWORD" \
  -e "CREATE DATABASE IF NOT EXISTS \`$DB_NAME\`;"

echo "==> applying schema $MIGRATION_FILE"
mysql -h "$RDS_HOST" -P "$RDS_PORT" -u "$RDS_USER" "-p$RDS_PASSWORD" "$DB_NAME" < "$MIGRATION_FILE"

echo "schema migration applied successfully"
