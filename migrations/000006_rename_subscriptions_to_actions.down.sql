ALTER TABLE delivery_attempts RENAME COLUMN action_id TO subscription_id;
ALTER TABLE actions DROP CONSTRAINT chk_js_script;
ALTER TABLE actions DROP CONSTRAINT chk_webhook_target;
ALTER TABLE actions DROP CONSTRAINT chk_action_type;
ALTER TABLE actions ALTER COLUMN target_url SET NOT NULL;
ALTER TABLE actions DROP COLUMN script_body;
ALTER TABLE actions DROP COLUMN type;
ALTER INDEX idx_actions_source RENAME TO idx_subscriptions_source;
ALTER TABLE actions RENAME TO subscriptions;
