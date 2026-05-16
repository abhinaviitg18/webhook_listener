package config

import "testing"

func TestLoadPrefersCommerceMySQLDSNOverTiDBDSN(t *testing.T) {
	t.Setenv("COMMERCE_MYSQL_DSN", "commerce-dsn")
	t.Setenv("TIDB_DSN", "tidb-dsn")
	t.Setenv("USE_IN_MEMORY_STORE", "false")

	cfg := Load()
	if cfg.TiDBDSN != "commerce-dsn" {
		t.Fatalf("expected COMMERCE_MYSQL_DSN to win, got %q", cfg.TiDBDSN)
	}
}

func TestLoadFallsBackToTiDBDSN(t *testing.T) {
	t.Setenv("COMMERCE_MYSQL_DSN", "")
	t.Setenv("TIDB_DSN", "tidb-dsn")
	t.Setenv("USE_IN_MEMORY_STORE", "false")

	cfg := Load()
	if cfg.TiDBDSN != "tidb-dsn" {
		t.Fatalf("expected TIDB_DSN fallback, got %q", cfg.TiDBDSN)
	}
}

func TestLoadLLMCompactionDefaults(t *testing.T) {
	t.Setenv("LLM_COMPACTION_ENABLED", "")
	t.Setenv("LLM_COMPACTION_THRESHOLD_BYTES", "")
	t.Setenv("LLM_COMPACTION_MAX_STRING_BYTES", "")
	t.Setenv("LLM_COMPACTION_MAX_ARRAY_ITEMS", "")
	t.Setenv("LLM_COMPACTION_MAX_OBJECT_FIELDS", "")

	cfg := Load()
	if !cfg.LLMCompactionEnabled {
		t.Fatalf("expected compaction enabled by default")
	}
	if cfg.LLMCompactionThresholdBytes != 6000 {
		t.Fatalf("expected threshold 6000, got %d", cfg.LLMCompactionThresholdBytes)
	}
	if cfg.LLMCompactionMaxStringBytes != 400 {
		t.Fatalf("expected max string 400, got %d", cfg.LLMCompactionMaxStringBytes)
	}
	if cfg.LLMCompactionMaxArrayItems != 8 {
		t.Fatalf("expected max array items 8, got %d", cfg.LLMCompactionMaxArrayItems)
	}
	if cfg.LLMCompactionMaxObjectFields != 20 {
		t.Fatalf("expected max object fields 20, got %d", cfg.LLMCompactionMaxObjectFields)
	}
}

func TestLoadLLMCompactionEnvOverrides(t *testing.T) {
	t.Setenv("LLM_COMPACTION_ENABLED", "false")
	t.Setenv("LLM_COMPACTION_THRESHOLD_BYTES", "7001")
	t.Setenv("LLM_COMPACTION_MAX_STRING_BYTES", "255")
	t.Setenv("LLM_COMPACTION_MAX_ARRAY_ITEMS", "5")
	t.Setenv("LLM_COMPACTION_MAX_OBJECT_FIELDS", "12")

	cfg := Load()
	if cfg.LLMCompactionEnabled {
		t.Fatalf("expected compaction disabled by override")
	}
	if cfg.LLMCompactionThresholdBytes != 7001 {
		t.Fatalf("expected threshold 7001, got %d", cfg.LLMCompactionThresholdBytes)
	}
	if cfg.LLMCompactionMaxStringBytes != 255 {
		t.Fatalf("expected max string 255, got %d", cfg.LLMCompactionMaxStringBytes)
	}
	if cfg.LLMCompactionMaxArrayItems != 5 {
		t.Fatalf("expected max array items 5, got %d", cfg.LLMCompactionMaxArrayItems)
	}
	if cfg.LLMCompactionMaxObjectFields != 12 {
		t.Fatalf("expected max object fields 12, got %d", cfg.LLMCompactionMaxObjectFields)
	}
}

func TestLoadLangfuseDefaults(t *testing.T) {
	t.Setenv("LANGFUSE_ENABLED", "")
	t.Setenv("LANGFUSE_HOST", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "")
	t.Setenv("LANGFUSE_SECRET_KEY", "")

	cfg := Load()
	if cfg.LangfuseEnabled {
		t.Fatalf("expected Langfuse disabled by default")
	}
	if cfg.LangfuseHost != "https://cloud.langfuse.com" {
		t.Fatalf("expected default Langfuse host, got %q", cfg.LangfuseHost)
	}
}

func TestLoadLangfuseOverrides(t *testing.T) {
	t.Setenv("LANGFUSE_ENABLED", "true")
	t.Setenv("LANGFUSE_HOST", "https://example.langfuse.test")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-test")

	cfg := Load()
	if !cfg.LangfuseEnabled {
		t.Fatalf("expected Langfuse enabled")
	}
	if cfg.LangfuseHost != "https://example.langfuse.test" {
		t.Fatalf("unexpected Langfuse host %q", cfg.LangfuseHost)
	}
	if cfg.LangfusePublicKey != "pk-test" || cfg.LangfuseSecretKey != "sk-test" {
		t.Fatalf("expected Langfuse keys to load from env")
	}
}

func TestLoadSingleTenantConfig(t *testing.T) {
	t.Setenv("APP_DEPLOYMENT_MODE", "single_tenant")
	t.Setenv("SINGLE_TENANT_OWNER_EMAIL", "ops@example.com")
	t.Setenv("SINGLE_TENANT_OWNER_ALIAS", "ops")
	t.Setenv("SINGLE_TENANT_SETUP_TOKEN_SHA256", "abc123")
	t.Setenv("ALLOW_PUBLIC_REGISTRATION", "true")

	cfg := Load()
	if cfg.AppDeploymentMode != "single_tenant" {
		t.Fatalf("expected single_tenant mode, got %q", cfg.AppDeploymentMode)
	}
	if cfg.SingleTenantOwnerEmail != "ops@example.com" || cfg.SingleTenantOwnerAlias != "ops" {
		t.Fatalf("unexpected owner config: %#v", cfg)
	}
	if cfg.SingleTenantSetupTokenSHA256 != "abc123" {
		t.Fatalf("unexpected setup token hash: %q", cfg.SingleTenantSetupTokenSHA256)
	}
	if !cfg.AllowPublicRegistration {
		t.Fatalf("expected public registration override")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid single-tenant config: %v", err)
	}
}

func TestValidateSingleTenantRequiresOwnerAndSetupToken(t *testing.T) {
	cfg := Config{Port: "8080", AppDeploymentMode: "single_tenant", UseInMemoryStore: true}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected missing single-tenant owner/setup token to fail")
	}
}
