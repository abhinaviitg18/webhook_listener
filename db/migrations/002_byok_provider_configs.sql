CREATE TABLE IF NOT EXISTS byok_provider_configs (
  id          VARCHAR(36) PRIMARY KEY,
  account_id  VARCHAR(36) NOT NULL,
  provider    VARCHAR(50) NOT NULL,
  api_key     TEXT NOT NULL,
  base_url    TEXT NOT NULL,
  model       VARCHAR(100) NOT NULL,
  is_default  BOOLEAN DEFAULT FALSE,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_account_provider (account_id, provider)
);
