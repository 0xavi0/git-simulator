#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-localhost}"
PORT="${2:-8080}"
WEBHOOK_TARGET="${3:?Usage: $0 HOST PORT WEBHOOK_TARGET [HEAD_DELAY_MS] [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
HEAD_DELAY_MS="${4:-0}"
VENDOR="${5:-github}"
OWNER="${6:-test-org}"
REPO="${7:-test-repo}"
BRANCH="${8:-main}"
SECRET="${9:-}"

BASE_URL="http://${HOST}:${PORT}"
REPO_PATH="${VENDOR}/${OWNER}/${REPO}"

SHA=$(curl -sf -X POST "${BASE_URL}/control/repos/${REPO_PATH}/commits" \
  -H "Content-Type: application/json" \
  -d "{\"branch\":\"${BRANCH}\",\"headDelayMs\":${HEAD_DELAY_MS}}" \
  | jq -r '.sha')

echo "commit SHA: ${SHA}"

curl -sf -X POST "${BASE_URL}/control/repos/${REPO_PATH}/webhooks" \
  -H "Content-Type: application/json" \
  -d "{\"sha\":\"${SHA}\",\"target\":\"${WEBHOOK_TARGET}\",\"branch\":\"${BRANCH}\",\"secret\":\"${SECRET}\"}" \
  | jq .
