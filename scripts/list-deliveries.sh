#!/usr/bin/env bash
# List recent deliveries, optionally filtered by source.
# Usage: ./scripts/list-deliveries.sh [source_slug] [limit]
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
SOURCE="${1:-}"
LIMIT="${2:-10}"

URL="$BASE_URL/deliveries?limit=$LIMIT"
[ -n "$SOURCE" ] && URL="$URL&source=$SOURCE"

curl -s "$URL" | jq .
