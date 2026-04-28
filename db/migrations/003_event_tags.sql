ALTER TABLE webhook_events ADD COLUMN IF NOT EXISTS tags_json TEXT NULL AFTER action_selected;
