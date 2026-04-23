#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "run as root (sudo)"
  exit 1
fi

APP_DIR="${APP_DIR:-/opt/agenthook/repo}"
APP_ENV_FILE="${APP_ENV_FILE:-/opt/agenthook/.env}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/agenthook/prod/env}"
APP_BIN="${APP_BIN:-/usr/local/bin/agenthook-server}"

if [[ ! -d "$APP_DIR" ]]; then
  echo "APP_DIR not found: $APP_DIR"
  exit 1
fi

mkdir -p /opt/agenthook
chown -R ec2-user:ec2-user /opt/agenthook

aws ssm get-parameter \
  --name "$APP_ENV_SSM_PARAM" \
  --with-decryption \
  --region "${AWS_REGION:-us-east-1}" \
  --query "Parameter.Value" \
  --output text > "$APP_ENV_FILE"
chmod 600 "$APP_ENV_FILE"
chown ec2-user:ec2-user "$APP_ENV_FILE"

cd "$APP_DIR"

# Build Backend
echo "Building backend..."
export CGO_ENABLED=0
sudo -u ec2-user /usr/bin/env bash -lc "go mod download"
sudo -u ec2-user /usr/bin/env bash -lc "go build -o /tmp/agenthook-server ./cmd/server"
mv /tmp/agenthook-server "$APP_BIN"
chmod 755 "$APP_BIN"

install -m 644 deploy/aws/agenthook.service /etc/systemd/system/agenthook.service
systemctl daemon-reload
systemctl enable agenthook
systemctl restart agenthook
systemctl --no-pager --full status agenthook | head -n 30
