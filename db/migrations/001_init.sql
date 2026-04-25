CREATE TABLE IF NOT EXISTS accounts (
  id VARCHAR(64) PRIMARY KEY,
  slug VARCHAR(128) NOT NULL UNIQUE,
  owner_email VARCHAR(320) NOT NULL,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS account_tokens (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  token_hash VARCHAR(128) NOT NULL,
  created_at DATETIME NOT NULL,
  revoked_at DATETIME NULL,
  INDEX idx_account_tokens_account (account_id),
  INDEX idx_account_tokens_hash (token_hash)
);

CREATE TABLE IF NOT EXISTS webhook_types (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  type_key VARCHAR(128) NOT NULL,
  plain_text_action VARCHAR(128) NULL,
  use_llm_fallback BOOLEAN NOT NULL DEFAULT FALSE,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uniq_account_type (account_id, type_key)
);

CREATE TABLE IF NOT EXISTS webhook_secrets (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  type_id VARCHAR(64) NOT NULL,
  secret_value VARCHAR(255) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_secret_value (secret_value)
);

CREATE TABLE IF NOT EXISTS forward_targets (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  config_json JSON NOT NULL,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_events (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  type_id VARCHAR(64) NOT NULL,
  secret_id VARCHAR(64) NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  source_event_id VARCHAR(128) NULL,
  type_key VARCHAR(128) NOT NULL,
  raw_payload_json JSON NULL,
  payload_json JSON NOT NULL,
  processed_text TEXT NULL,
  action_selected VARCHAR(128) NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_events_account_created (account_id, created_at),
  INDEX idx_events_request (request_id),
  UNIQUE KEY uq_events_account_source (account_id, source_event_id)
);

ALTER TABLE webhook_events
  ADD COLUMN IF NOT EXISTS source_event_id VARCHAR(128) NULL AFTER request_id;

ALTER TABLE webhook_events
  ADD COLUMN IF NOT EXISTS raw_payload_json JSON NULL AFTER type_key;

ALTER TABLE webhook_events
  ADD COLUMN IF NOT EXISTS processed_text TEXT NULL AFTER payload_json;

CREATE UNIQUE INDEX IF NOT EXISTS uq_events_account_source
  ON webhook_events (account_id, source_event_id);

CREATE TABLE IF NOT EXISTS webhook_type_signatures (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  type_key VARCHAR(128) NOT NULL,
  version INT NOT NULL,
  required_keys_json JSON NOT NULL,
  shape_hints_json JSON NOT NULL,
  header_hints_json JSON NOT NULL,
  confidence_threshold DOUBLE NOT NULL DEFAULT 0.75,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  source VARCHAR(64) NOT NULL DEFAULT 'manual',
  created_at DATETIME NOT NULL,
  INDEX idx_sig_account_type (account_id, type_key, enabled)
);

CREATE TABLE IF NOT EXISTS webhook_transforms (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  type_key VARCHAR(128) NOT NULL,
  version INT NOT NULL,
  engine VARCHAR(32) NOT NULL,
  wasm_blob_ref VARCHAR(1024) NULL,
  dsl_text JSON NULL,
  deterministic_tests_json JSON NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_transform_active (account_id, type_key, status, version)
);

CREATE TABLE IF NOT EXISTS webhook_transform_runs (
  id VARCHAR(64) PRIMARY KEY,
  event_id VARCHAR(64) NULL,
  account_id VARCHAR(64) NOT NULL,
  type_key VARCHAR(128) NOT NULL,
  transform_version INT NOT NULL,
  duration_ms BIGINT NOT NULL,
  result_hash VARCHAR(128) NULL,
  error_text TEXT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_runs_account_type (account_id, type_key, created_at)
);

CREATE TABLE IF NOT EXISTS master_prompt_policies (
  account_id VARCHAR(64) PRIMARY KEY,
  prompt_text TEXT NOT NULL,
  updated_by VARCHAR(320) NOT NULL,
  updated_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_skills (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  type_key VARCHAR(128) NOT NULL,
  skill_key VARCHAR(128) NOT NULL,
  skill_prompt TEXT NOT NULL,
  match_contains TEXT NOT NULL,
  forced_action VARCHAR(128) NULL,
  memory_write_mode VARCHAR(32) NOT NULL DEFAULT 'update_or_insert',
  priority INT NOT NULL DEFAULT 100,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME NOT NULL,
  INDEX idx_skill_account_type (account_id, type_key, enabled, priority),
  UNIQUE KEY uniq_skill (account_id, type_key, skill_key)
);

CREATE TABLE IF NOT EXISTS webhook_autopromote_states (
  account_id VARCHAR(64) NOT NULL,
  type_key VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  validated_count INT NOT NULL DEFAULT 0,
  shadow_total INT NOT NULL DEFAULT 0,
  shadow_success INT NOT NULL DEFAULT 0,
  last_confidence DOUBLE NOT NULL DEFAULT 0,
  last_reason VARCHAR(512) NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (account_id, type_key)
);
