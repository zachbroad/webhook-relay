#!/usr/bin/env bash
# Fire a webhook at a source.
# Usage: ./scripts/send-webhook.sh [source_slug] [json_payload]
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
SOURCE="${1:-my-app}"
PAYLOAD="${2:-'{"event":"ping","ts":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}'}"

echo "POST $BASE_URL/webhooks/$SOURCE"
curl -s -X POST "$BASE_URL/webhooks/$SOURCE" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" | jq .
