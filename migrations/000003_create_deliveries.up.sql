CREATE TYPE delivery_status AS ENUM ('pending', 'processing', 'completed', 'failed');

CREATE TABLE deliveries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id       UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    headers         JSONB NOT NULL DEFAULT '{}',
    payload         JSONB NOT NULL,
    status          delivery_status NOT NULL DEFAULT 'pending',
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_id, idempotency_key)
);

CREATE INDEX idx_deliveries_status ON deliveries (status) WHERE status IN ('pending', 'processing');
