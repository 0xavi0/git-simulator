#!/usr/bin/env bash
set -euo pipefail

GITREPO_NAME="${1:?Usage: $0 GITREPO_NAME NAMESPACE HEAD_DELAY_MS HOST PORT WEBHOOK_TARGET [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
NAMESPACE="${2:?Usage: $0 GITREPO_NAME NAMESPACE HEAD_DELAY_MS HOST PORT WEBHOOK_TARGET [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
HEAD_DELAY_MS="${3:-0}"
HOST="${4:-localhost}"
PORT="${5:-8080}"
WEBHOOK_TARGET="${6:?Usage: $0 GITREPO_NAME NAMESPACE HEAD_DELAY_MS HOST PORT WEBHOOK_TARGET [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
VENDOR="${7:-github}"
OWNER="${8:-test-org}"
REPO="${9:-test-repo}"
BRANCH="${10:-main}"
SECRET="${11:-}"

BASE_URL="http://${HOST}:${PORT}"
REPO_PATH="${VENDOR}/${OWNER}/${REPO}"

POLL_INTERVAL=2
POLL_TIMEOUT=60

wait_for_field() {
  local field="$1"
  local expected="$2"
  local elapsed=0
  while [[ $elapsed -lt $POLL_TIMEOUT ]]; do
    actual=$(kubectl get gitrepo "${GITREPO_NAME}" -n "${NAMESPACE}" \
      -o jsonpath="{${field}}" 2>/dev/null || true)
    if [[ "${actual}" == "${expected}" ]]; then
      return 0
    fi
    sleep "${POLL_INTERVAL}"
    elapsed=$((elapsed + POLL_INTERVAL))
  done
  return 1
}

iteration=0
while true; do
  iteration=$((iteration + 1))
  echo ""
  echo "=== Iteration ${iteration} (head delay: ${HEAD_DELAY_MS}ms) ==="

  SHA=$(curl -sf -X POST "${BASE_URL}/control/repos/${REPO_PATH}/commits" \
    -H "Content-Type: application/json" \
    -d "{\"branch\":\"${BRANCH}\",\"headDelayMs\":${HEAD_DELAY_MS}}" \
    | jq -r '.sha')
  echo "commit SHA: ${SHA}"

  curl -sf -X POST "${BASE_URL}/control/repos/${REPO_PATH}/webhooks" \
    -H "Content-Type: application/json" \
    -d "{\"sha\":\"${SHA}\",\"target\":\"${WEBHOOK_TARGET}\",\"branch\":\"${BRANCH}\",\"secret\":\"${SECRET}\"}" \
    | jq .

  if wait_for_field ".status.webhookCommit" "${SHA}"; then
    echo "OK for webhook"
  else
    echo "FAIL: .status.webhookCommit did not reach ${SHA} within ${POLL_TIMEOUT}s"
    exit 1
  fi

  if wait_for_field ".status.commit" "${SHA}"; then
    echo "OK for commit"
  else
    echo "FAIL: .status.commit did not reach ${SHA} within ${POLL_TIMEOUT}s"
    exit 1
  fi
done
