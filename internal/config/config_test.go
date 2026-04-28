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
