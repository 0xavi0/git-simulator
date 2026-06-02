#!/usr/bin/env bash
# curl_scenarios.sh — control-API examples for the git-simulator binary.
#
# Prerequisites:
#   gitsim --listen :8080 --vendors github &
#
# All commands assume BASE=http://localhost:8080.
# Replace RECEIVER_URL with the URL of Fleet's gitjob webhook receiver
# (e.g. from kubectl port-forward).

set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
RECEIVER_URL="${RECEIVER_URL:-http://localhost:9090}"

echo "=== Health check ==="
curl -sf "$BASE/healthz"
echo

# ─── Create a repo ────────────────────────────────────────────────────────────
echo "=== Create repo ==="
REPO=$(curl -sf -X POST "$BASE/control/repos" \
  -H 'Content-Type: application/json' \
  -d '{"vendor":"github","owner":"acme","repo":"myapp","defaultBranch":"main"}')
echo "$REPO" | python3 -m json.tool
CLONE_URL=$(echo "$REPO" | python3 -c "import sys,json; print(json.load(sys.stdin)['cloneURL'])")
echo "Clone URL: $CLONE_URL"

# ─── Push a commit ────────────────────────────────────────────────────────────
echo
echo "=== Push a commit ==="
COMMIT=$(curl -sf -X POST "$BASE/control/repos/github/acme/myapp/commits" \
  -H 'Content-Type: application/json' \
  -d '{"branch":"main"}')
echo "$COMMIT" | python3 -m json.tool
SHA=$(echo "$COMMIT" | python3 -c "import sys,json; print(json.load(sys.stdin)['sha'])")
echo "New SHA: $SHA"

# ─── Fire a webhook ───────────────────────────────────────────────────────────
echo
echo "=== Fire webhook ==="
curl -sf -X POST "$BASE/control/repos/github/acme/myapp/webhooks" \
  -H 'Content-Type: application/json' \
  -d "{\"sha\":\"$SHA\",\"target\":\"$RECEIVER_URL\",\"secret\":\"fleet-secret\"}" \
  | python3 -m json.tool

# ─── Scenario: push-N (3 commits, ascending webhook order) ───────────────────
echo
echo "=== Scenario: push-N (3 commits, asc) ==="
curl -sf -X POST "$BASE/control/scenarios/push-n" \
  -H 'Content-Type: application/json' \
  -d "{
    \"repoID\": \"github/acme/myapp\",
    \"n\": 3,
    \"order\": \"asc\",
    \"target\": \"$RECEIVER_URL\",
    \"secret\": \"fleet-secret\"
  }" | python3 -m json.tool

# ─── Scenario: push-N (newest-first / out-of-order) ──────────────────────────
echo
echo "=== Scenario: push-N (3 commits, desc) ==="
curl -sf -X POST "$BASE/control/scenarios/push-n" \
  -H 'Content-Type: application/json' \
  -d "{
    \"repoID\": \"github/acme/myapp\",
    \"n\": 3,
    \"order\": \"desc\",
    \"target\": \"$RECEIVER_URL\"
  }" | python3 -m json.tool

# ─── Scenario: webhook-before-HEAD race ──────────────────────────────────────
# The simulator fires the webhook for the new commit while the remote's advertised
# HEAD is still the previous commit for headDelayMs milliseconds.
# (The ManualClock is only available in-process; headDelayMs here uses the real clock.)
echo
echo "=== Scenario: race (webhook before HEAD) ==="
curl -sf -X POST "$BASE/control/scenarios/race" \
  -H 'Content-Type: application/json' \
  -d "{
    \"repoID\":      \"github/acme/myapp\",
    \"headDelayMs\": 200,
    \"target\":      \"$RECEIVER_URL\",
    \"secret\":      \"fleet-secret\"
  }" | python3 -m json.tool

# ─── Scenario: mixed (per-commit HEAD delays and send-after timings) ─────────
echo
echo "=== Scenario: mixed ==="
curl -sf -X POST "$BASE/control/scenarios/mixed" \
  -H 'Content-Type: application/json' \
  -d "{
    \"repoID\": \"github/acme/myapp\",
    \"commits\": [
      {\"headDelayMs\": 200, \"sendAfterMs\": 0},
      {\"headDelayMs\":   0, \"sendAfterMs\": 0},
      {\"headDelayMs\": 100, \"sendAfterMs\": 50}
    ],
    \"target\": \"$RECEIVER_URL\"
  }" | python3 -m json.tool

# ─── ls-remote (git protocol, no curl — use git or go-git) ───────────────────
echo
echo "=== ls-remote ==="
git ls-remote "$CLONE_URL" HEAD
