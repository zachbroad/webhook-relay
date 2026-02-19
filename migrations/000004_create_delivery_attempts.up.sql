CREATE TYPE attempt_status AS ENUM ('pending', 'success', 'failed');

CREATE TABLE delivery_attempts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id     UUID NOT NULL REFERENCES deliveries(id) ON DELETE CASCADE,
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    attempt_number  INT NOT NULL DEFAULT 1,
    status          attempt_status NOT NULL DEFAULT 'pending',
    response_status INT,
    response_body   TEXT,
    error_message   TEXT,
    next_retry_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (delivery_id, subscription_id, attempt_number)
);

CREATE INDEX idx_attempts_retry ON delivery_attempts (next_retry_at)
    WHERE status = 'failed' AND next_retry_at IS NOT NULL;
