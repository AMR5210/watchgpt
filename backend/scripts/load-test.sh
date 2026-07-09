#!/usr/bin/env bash
set -euo pipefail

REQUESTS="${REQUESTS:-10000}"
CONCURRENCY="${CONCURRENCY:-50}"
URL="${URL:-}"
METHOD="${METHOD:-GET}"
BODY="${BODY:-}"
TOKEN="${TOKEN:-}"
export GOCACHE="${GOCACHE:-/tmp/watchgpt-gocache}"

args=(-n "$REQUESTS" -c "$CONCURRENCY" -method "$METHOD")

if [[ -n "$URL" ]]; then
  args+=(-url "$URL")
fi

if [[ -n "$BODY" ]]; then
  args+=(-body "$BODY" -header "Content-Type: application/json")
fi

if [[ -n "$TOKEN" ]]; then
  args+=(-header "Authorization: Bearer $TOKEN")
fi

go run ./cmd/loadtest "${args[@]}"
