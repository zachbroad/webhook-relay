ALTER TABLE deliveries
    DROP COLUMN transformed_payload,
    DROP COLUMN transformed_headers;

ALTER TABLE sources
    DROP COLUMN mode,
    DROP COLUMN script_body;

-- Note: Cannot remove enum value 'recorded' from delivery_status in PostgreSQL.
