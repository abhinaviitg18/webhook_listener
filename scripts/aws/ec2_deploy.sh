#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "run as root (sudo)"
  exit 1
fi

APP_DIR="${APP_DIR:-/opt/hookweb/repo}"
APP_ENV_FILE="${APP_ENV_FILE:-/opt/hookweb/.env}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/hookweb/prod/env}"
APP_BIN="${APP_BIN:-/usr/local/bin/hookweb-server}"

if [[ ! -d "$APP_DIR" ]]; then
  echo "APP_DIR not found: $APP_DIR"
  exit 1
fi

mkdir -p /opt/hookweb
chown -R ec2-user:ec2-user /opt/hookweb

aws ssm get-parameter \
  --name "$APP_ENV_SSM_PARAM" \
  --with-decryption \
  --region "${AWS_REGION:-us-east-1}" \
  --query "Parameter.Value" \
  --output text > "$APP_ENV_FILE"
chmod 600 "$APP_ENV_FILE"
chown ec2-user:ec2-user "$APP_ENV_FILE"

cd "$APP_DIR"

# Build Frontend
echo "Building frontend..."
cd web
sudo -u ec2-user /usr/bin/env bash -lc "npm install && npm run build"
cd ..
mkdir -p internal/ui/dist
cp -r web/dist/* internal/ui/dist/

# Build Backend
echo "Building backend..."
export CGO_ENABLED=0
sudo -u ec2-user /usr/bin/env bash -lc "go mod download"
sudo -u ec2-user /usr/bin/env bash -lc "go build -o /tmp/hookweb-server ./cmd/server"
mv /tmp/hookweb-server "$APP_BIN"
chmod 755 "$APP_BIN"

install -m 644 deploy/aws/hookweb.service /etc/systemd/system/hookweb.service
systemctl daemon-reload
systemctl enable hookweb
systemctl restart hookweb
systemctl --no-pager --full status hookweb | head -n 30
