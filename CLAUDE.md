# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Webhook relay/fan-out service in Go. Receives incoming webhooks via HTTP, stores them in Postgres, publishes to a Redis Stream, and a worker fans out deliveries to registered actions (webhook endpoints or JavaScript scripts) with retry logic, idempotency, and HMAC signing.

## Build & Run Commands

```bash
make build              # Build api + worker binaries to bin/
make run-api            # go run ./cmd/api
make run-worker         # go run ./cmd/worker
make test               # go test ./...
go test ./internal/signing/  # Run a single package's tests

make docker-build       # docker compose build
make docker-up          # docker compose up -d (Postgres, Redis, API, 2 workers)
make docker-down        # docker compose down

make migrate-up         # Run migrations (requires golang-migrate CLI)
make migrate-down       # Rollback migrations
make migrate-create     # Create new migration (prompts for name)
```

## Architecture

Two entry points in `cmd/`:
- **`cmd/api`** — HTTP server (:8080). Ingests webhooks, manages actions, lists deliveries. Pass `--worker` to also run the fan-out worker in-process (used by `air` for local dev).
- **`cmd/worker`** — Redis Stream consumer + retry poller. Fans out to actions. Health endpoint on :8081.

**Flow:** Webhook POST → API stores delivery (Postgres, status=pending) → XADD to Redis Stream `deliveries` → Worker XREADGROUP → dispatch to each active action (HTTP POST for webhook type, JS execution for javascript type) → Record delivery_attempts → Retry failed attempts with exponential backoff + jitter.

**Key packages under `internal/`:**
- `config` — Loads all config from environment variables
- `database` — pgxpool connection setup
- `handler` — HTTP handlers (webhook ingest, action CRUD, delivery listing)
- `model` — Domain types: Source, Action (with type: webhook|javascript), Delivery, DeliveryAttempt
- `script` — Transform scripts (source-level) and action scripts (per-action JS via goja)
- `signing` — HMAC-SHA256 sign/verify (mirrors GitHub's `X-Webhook-Signature-256` scheme)
- `store` — Data access layer with raw SQL via pgx (no ORM)
- `worker` — FanoutWorker: stream consumer, catch-up poller, retry poller

## Database

Four tables via golang-migrate migrations in `migrations/`:
- `sources` — Webhook event sources (seeded via SQL, no create API)
- `actions` — Per-source actions with `type` (webhook or javascript), optional `target_url`, optional `script_body`, optional `signing_secret`
- `deliveries` — One per incoming webhook, deduplicated by `(source_id, idempotency_key)`
- `delivery_attempts` — Per-action delivery attempt with retry tracking

## Action Types

- **webhook** — HTTP POST to `target_url` with optional HMAC signing
- **javascript** — Runs a `process(event)` function via goja JS runtime; result stored in delivery attempt

## Key Design Details

- Sources must be seeded directly via SQL (`scripts/seed-source.sh`); no API endpoint for creating them.
- Redis Stream `deliveries` uses consumer group `fanout-workers` with blocking XREADGROUP (5s), manual XACK/XDEL, capped at ~10k messages.
- Catch-up poller (default 30s) reprocesses `pending` deliveries missed by the stream.
- Retry poller reprocesses failed attempts with exponential backoff (base 5s, cap 5min, +/-25% jitter, max 5 retries).
- No authentication on API endpoints.
- `X-Idempotency-Key` header for deduplication (auto-generates UUID if absent).

## Environment Variables

Key config (see `internal/config/config.go`): `DATABASE_URL`, `REDIS_URL`, `PORT`, `WORKER_CONCURRENCY`, `MAX_RETRIES`, `RETRY_BASE_DELAY`, `DELIVERY_TIMEOUT`, `POLL_INTERVAL`. Defaults are suitable for local dev with docker-compose.

## Dependencies

Go 1.24+, gin (router), pgx (Postgres), go-redis (Redis streams), goja (JS runtime), google/uuid, godotenv. External tools: docker compose, golang-migrate CLI, psql/jq (for shell scripts in `scripts/`).
