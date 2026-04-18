#!/usr/bin/env bash
set -euo pipefail
if ! command -v tiup >/dev/null 2>&1; then
  echo "tiup is required. Install from https://docs.pingcap.com/tidb/stable/tiup-overview"
  exit 1
fi

echo "Starting local TiDB playground on 127.0.0.1:4000"
exec tiup playground --tag hookweb --db 1 --kv 1 --pd 1
