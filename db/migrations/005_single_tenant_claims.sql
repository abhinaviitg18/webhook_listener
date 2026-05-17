CREATE TABLE IF NOT EXISTS single_tenant_claims (
  id VARCHAR(64) PRIMARY KEY,
  owner_email VARCHAR(320) NOT NULL,
  claim_hash VARCHAR(128) NOT NULL,
  created_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  consumed_at DATETIME NULL,
  consumed_account_id VARCHAR(64) NULL,
  UNIQUE KEY uq_single_tenant_claim_hash (claim_hash),
  INDEX idx_single_tenant_claim_owner_active (owner_email, consumed_at, expires_at)
);
