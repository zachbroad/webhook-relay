#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DB_URL="${DATABASE_URL:-postgres://relay:relay@localhost:5432/webhook_relay?sslmode=disable}"
SOURCE_SLUG="my-app"

# Colors
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

step() { echo -e "\n${CYAN}==> $1${NC}"; }
ok()   { echo -e "${GREEN}    ✓ $1${NC}"; }
info() { echo -e "${YELLOW}    $1${NC}"; }

# ─── Health check ───────────────────────────────────────────────
step "Health check"
curl -sf "$BASE_URL/healthz"
echo
ok "API is up"

# ─── Seed a source (idempotent) ─────────────────────────────────
step "Seeding source '$SOURCE_SLUG' in Postgres"
psql "$DB_URL" -qc "
  INSERT INTO sources (name, slug)
  VALUES ('My App', '$SOURCE_SLUG')
  ON CONFLICT (slug) DO NOTHING;
"
ok "Source ready"

# ─── Create a subscription ──────────────────────────────────────
step "Creating subscription → https://httpbin.org/post"
SUB_RESPONSE=$(curl -sf -X POST "$BASE_URL/sources/$SOURCE_SLUG/subscriptions" \
  -H "Content-Type: application/json" \
  -d '{"target_url": "https://httpbin.org/post"}')
echo "$SUB_RESPONSE" | jq .
SUB_ID=$(echo "$SUB_RESPONSE" | jq -r '.id')
ok "Subscription created: $SUB_ID"

# ─── List subscriptions ─────────────────────────────────────────
step "Listing subscriptions for '$SOURCE_SLUG'"
curl -sf "$BASE_URL/sources/$SOURCE_SLUG/subscriptions" | jq .

# ─── Send a webhook ─────────────────────────────────────────────
step "Sending webhook to '$SOURCE_SLUG'"
WEBHOOK_RESPONSE=$(curl -sf -X POST "$BASE_URL/webhooks/$SOURCE_SLUG" \
  -H "Content-Type: application/json" \
  -d '{"event": "order.created", "data": {"order_id": 42, "amount": 99.99}}')
echo "$WEBHOOK_RESPONSE" | jq .
DELIVERY_ID=$(echo "$WEBHOOK_RESPONSE" | jq -r '.delivery_id')
ok "Delivery queued: $DELIVERY_ID"

# ─── Wait for worker to process ─────────────────────────────────
step "Waiting 3s for worker to pick it up..."
sleep 3

# ─── Check deliveries ───────────────────────────────────────────
step "Listing recent deliveries"
curl -sf "$BASE_URL/deliveries?source=$SOURCE_SLUG&limit=5" | jq .

# ─── Cleanup: delete subscription ───────────────────────────────
step "Cleaning up: deleting subscription $SUB_ID"
curl -sf -X DELETE "$BASE_URL/sources/$SOURCE_SLUG/subscriptions/$SUB_ID"
ok "Subscription deleted"

echo -e "\n${GREEN}All done!${NC}"
