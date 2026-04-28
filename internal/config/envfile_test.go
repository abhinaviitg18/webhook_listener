package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFilesPrefersExistingProcessEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.env")
	if err := os.WriteFile(path, []byte("LANGFUSE_ENABLED=true\nLANGFUSE_HOST=https://cloud.langfuse.com\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("LANGFUSE_ENABLED", "false")
	if err := LoadEnvFiles(path); err != nil {
		t.Fatalf("LoadEnvFiles: %v", err)
	}
	if got := os.Getenv("LANGFUSE_ENABLED"); got != "false" {
		t.Fatalf("expected process env to win, got %q", got)
	}
	if got := os.Getenv("LANGFUSE_HOST"); got != "https://cloud.langfuse.com" {
		t.Fatalf("expected missing env to be loaded, got %q", got)
	}
}
