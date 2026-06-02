#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-localhost}"
PORT="${2:-8080}"
VENDOR="${3:-github}"
OWNER="${4:-test-org}"
REPO="${5:-test-repo}"

BASE_URL="http://${HOST}:${PORT}"

curl -sf -X POST "${BASE_URL}/control/repos" \
  -H "Content-Type: application/json" \
  -d "{\"vendor\":\"${VENDOR}\",\"owner\":\"${OWNER}\",\"repo\":\"${REPO}\"}" \
  | jq -r '.cloneURL'
