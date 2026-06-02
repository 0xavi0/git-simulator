#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-localhost}"
PORT="${2:-8080}"
CONTENT_DIR="${3:-}"

LISTEN="${HOST}:${PORT}"
BASE_URL="http://${HOST}:${PORT}"

ARGS=(--listen "${LISTEN}" --git-base-url "${BASE_URL}")
if [[ -n "${CONTENT_DIR}" ]]; then
  ARGS+=(--content-dir "${CONTENT_DIR}")
fi

go run ./cmd/gitsim "${ARGS[@]}"
