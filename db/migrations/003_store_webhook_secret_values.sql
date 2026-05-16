ALTER TABLE webhook_secrets
  ADD COLUMN IF NOT EXISTS secret_value VARCHAR(255) NULL AFTER type_id;

CREATE INDEX IF NOT EXISTS idx_secret_value
  ON webhook_secrets (secret_value);
