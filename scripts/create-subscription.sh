#!/usr/bin/env bash
# Create a subscription for a source.
# Usage: ./scripts/create-subscription.sh [source_slug] [target_url]
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
SOURCE="${1:-my-app}"
TARGET="${2:-https://httpbin.org/post}"

echo "Creating subscription: $SOURCE → $TARGET"
curl -s -X POST "$BASE_URL/sources/$SOURCE/subscriptions" \
  -H "Content-Type: application/json" \
  -d "{\"target_url\": \"$TARGET\"}" | jq .
