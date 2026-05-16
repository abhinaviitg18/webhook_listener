package store

import "testing"

func TestNormalizeMySQLDSNLeavesDriverDSNUnchanged(t *testing.T) {
	dsn := "user:pass@tcp(example.internal:3306)/railway?parseTime=true"
	if got := NormalizeMySQLDSN(dsn); got != dsn {
		t.Fatalf("expected DSN unchanged, got %q", got)
	}
}

func TestNormalizeMySQLDSNConvertsRailwayMySQLURL(t *testing.T) {
	got := NormalizeMySQLDSN("mysql://root:secret@mysql.railway.internal:3306/railway")
	want := "root:secret@tcp(mysql.railway.internal:3306)/railway?parseTime=true"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeMySQLDSNPreservesURLQueryParams(t *testing.T) {
	got := NormalizeMySQLDSN("mysql://root:secret@mysql.railway.internal:3306/railway?charset=utf8mb4&tls=true")
	if got != "root:secret@tcp(mysql.railway.internal:3306)/railway?parseTime=true&charset=utf8mb4&tls=true" {
		t.Fatalf("unexpected normalized DSN: %q", got)
	}
}
