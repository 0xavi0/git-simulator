#!/usr/bin/env bash
set -euo pipefail

GITREPO_NAME="${1:?Usage: $0 GITREPO_NAME NAMESPACE MAX_HEAD_DELAY_MS MAX_COMMITS HOST PORT WEBHOOK_TARGET [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
NAMESPACE="${2:?Usage: $0 GITREPO_NAME NAMESPACE MAX_HEAD_DELAY_MS MAX_COMMITS HOST PORT WEBHOOK_TARGET [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
MAX_HEAD_DELAY_MS="${3:-0}"
MAX_COMMITS="${4:-5}"
HOST="${5:-localhost}"
PORT="${6:-8080}"
WEBHOOK_TARGET="${7:?Usage: $0 GITREPO_NAME NAMESPACE MAX_HEAD_DELAY_MS MAX_COMMITS HOST PORT WEBHOOK_TARGET [VENDOR] [OWNER] [REPO] [BRANCH] [SECRET]}"
VENDOR="${8:-github}"
OWNER="${9:-test-org}"
REPO="${10:-test-repo}"
BRANCH="${11:-main}"
SECRET="${12:-}"

BASE_URL="http://${HOST}:${PORT}"
REPO_PATH="${VENDOR}/${OWNER}/${REPO}"

POLL_INTERVAL=2
POLL_TIMEOUT=60

shuffle() {
  local arr=("$@")
  local n=${#arr[@]}
  for ((i = n - 1; i > 0; i--)); do
    local j=$(( RANDOM % (i + 1) ))
    local tmp="${arr[i]}"
    arr[i]="${arr[j]}"
    arr[j]="$tmp"
  done
  echo "${arr[@]}"
}

wait_for_commit() {
  local expected="$1"
  local elapsed=0
  while [[ $elapsed -lt $POLL_TIMEOUT ]]; do
    actual=$(kubectl get gitrepo "${GITREPO_NAME}" -n "${NAMESPACE}" \
      -o jsonpath="{.status.commit}" 2>/dev/null || true)
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
  n=$(( RANDOM % MAX_COMMITS + 1 ))
  echo ""
  echo "=== Iteration ${iteration} (${n} commits) ==="

  shas=()
  for ((i = 1; i <= n; i++)); do
    if [[ "${MAX_HEAD_DELAY_MS}" -gt 0 ]]; then
      delay=$(( RANDOM % (MAX_HEAD_DELAY_MS + 1) ))
    else
      delay=0
    fi
    sha=$(curl -sf -X POST "${BASE_URL}/control/repos/${REPO_PATH}/commits" \
      -H "Content-Type: application/json" \
      -d "{\"branch\":\"${BRANCH}\",\"headDelayMs\":${delay}}" \
      | jq -r '.sha')
    if [[ $i -eq $n ]]; then
      echo "  commit ${i}/${n}: ${sha} (head delay: ${delay}ms)  <-- HEAD"
    else
      echo "  commit ${i}/${n}: ${sha} (head delay: ${delay}ms)"
    fi
    shas+=("${sha}")
  done

  head_sha="${shas[${#shas[@]}-1]}"
  echo "HEAD SHA: ${head_sha}"

  shuffled=($(shuffle "${shas[@]}"))
  echo "webhook order: ${shuffled[*]}"

  for sha in "${shuffled[@]}"; do
    curl -sf -X POST "${BASE_URL}/control/repos/${REPO_PATH}/webhooks" \
      -H "Content-Type: application/json" \
      -d "{\"sha\":\"${sha}\",\"target\":\"${WEBHOOK_TARGET}\",\"branch\":\"${BRANCH}\",\"secret\":\"${SECRET}\"}" \
      > /dev/null
    echo "  webhook sent: ${sha}"
  done

  if wait_for_commit "${head_sha}"; then
    actual=$(kubectl get gitrepo "${GITREPO_NAME}" -n "${NAMESPACE}" \
      -o jsonpath="{.status.commit}" 2>/dev/null || true)
    echo "OK for commit (.status.commit: ${actual})"
  else
    actual=$(kubectl get gitrepo "${GITREPO_NAME}" -n "${NAMESPACE}" \
      -o jsonpath="{.status.commit}" 2>/dev/null || true)
    echo "FAIL: .status.commit is ${actual}, expected ${head_sha} (not reached within ${POLL_TIMEOUT}s)"
    exit 1
  fi
done
