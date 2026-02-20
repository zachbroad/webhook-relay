#!/usr/bin/env bash
# Fire N webhooks rapidly to load-test the relay.
# Usage: ./scripts/spam-webhooks.sh [count] [source_slug]
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
COUNT="${1:-10}"
SOURCE="${2:-my-app}"

echo "Sending $COUNT webhooks to $SOURCE..."
for i in $(seq 1 "$COUNT"); do
  curl -s -X POST "$BASE_URL/webhooks/$SOURCE" \
    -H "Content-Type: application/json" \
    -d "{\"event\":\"load-test\",\"seq\":$i}" &
done
wait
echo "Done — fired $COUNT webhooks."
