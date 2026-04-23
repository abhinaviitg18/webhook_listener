#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 1 ]]; then
  echo "usage: $0 <classify|transform> [args...]" >&2
  exit 2
fi
exec go run -mod=mod ./cmd/agenthook "$@"
