#!/usr/bin/env bash
# Create a webhook action for a source.
# Usage: ./scripts/create-action.sh [source_slug] [target_url]
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
SOURCE="${1:-my-app}"
TARGET="${2:-https://httpbin.org/post}"

echo "Creating action: $SOURCE → $TARGET"
curl -s -X POST "$BASE_URL/sources/$SOURCE/actions" \
  -H "Content-Type: application/json" \
  -d "{\"type\": \"webhook\", \"target_url\": \"$TARGET\"}" | jq .
