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
