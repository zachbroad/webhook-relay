ALTER TABLE sources
    ADD COLUMN mode TEXT NOT NULL DEFAULT 'active' CHECK (mode IN ('record', 'active')),
    ADD COLUMN script_body TEXT;

ALTER TYPE delivery_status ADD VALUE IF NOT EXISTS 'recorded';

ALTER TABLE deliveries
    ADD COLUMN transformed_payload JSONB,
    ADD COLUMN transformed_headers JSONB;
