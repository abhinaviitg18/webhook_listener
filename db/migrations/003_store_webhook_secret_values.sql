ALTER TABLE webhook_secrets
  ADD COLUMN secret_value VARCHAR(255) NULL AFTER type_id;

CREATE INDEX idx_secret_value
  ON webhook_secrets (secret_value);
