ALTER TABLE subscriptions RENAME TO actions;
ALTER INDEX idx_subscriptions_source RENAME TO idx_actions_source;
ALTER TABLE actions ADD COLUMN type TEXT NOT NULL DEFAULT 'webhook';
ALTER TABLE actions ADD COLUMN script_body TEXT;
ALTER TABLE actions ALTER COLUMN target_url DROP NOT NULL;
ALTER TABLE actions ADD CONSTRAINT chk_action_type CHECK (type IN ('webhook', 'javascript'));
ALTER TABLE actions ADD CONSTRAINT chk_webhook_target CHECK (type != 'webhook' OR target_url IS NOT NULL);
ALTER TABLE actions ADD CONSTRAINT chk_js_script CHECK (type != 'javascript' OR script_body IS NOT NULL);
ALTER TABLE delivery_attempts RENAME COLUMN subscription_id TO action_id;
