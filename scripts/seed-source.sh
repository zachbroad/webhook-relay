#!/usr/bin/env bash
# Seed a webhook source into Postgres.
# Usage: ./scripts/seed-source.sh [slug] [name]
set -euo pipefail

DB_URL="${DATABASE_URL:-postgres://relay:relay@localhost:5432/webhook_relay?sslmode=disable}"
SLUG="${1:-my-app}"
NAME="${2:-My App}"

psql "$DB_URL" -c "
  INSERT INTO sources (name, slug)
  VALUES ('$NAME', '$SLUG')
  ON CONFLICT (slug) DO NOTHING;
"
echo "Source '$SLUG' ready."
