package store

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"

	"agenthook.store/db/migrations"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMigrationStatementsCreateMissingColumnAndIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM information_schema.columns")).
		WithArgs("webhook_secrets", "secret_value").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE `webhook_secrets` ADD COLUMN `secret_value` VARCHAR(255) NULL AFTER type_id")).
		WillReturnResult(driver.RowsAffected(1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM information_schema.statistics")).
		WithArgs("webhook_secrets", "idx_secret_value").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta("CREATE INDEX `idx_secret_value` ON `webhook_secrets` (secret_value)")).
		WillReturnResult(driver.RowsAffected(1))

	err = applyMigrationStatements(context.Background(), db, `
ALTER TABLE webhook_secrets
  ADD COLUMN IF NOT EXISTS secret_value VARCHAR(255) NULL AFTER type_id;

CREATE INDEX IF NOT EXISTS idx_secret_value
  ON webhook_secrets (secret_value);
`)
	if err != nil {
		t.Fatalf("applyMigrationStatements: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationStatementsSkipExistingColumnAndIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM information_schema.columns")).
		WithArgs("webhook_secrets", "secret_value").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM information_schema.statistics")).
		WithArgs("webhook_secrets", "idx_secret_value").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err = applyMigrationStatements(context.Background(), db, `
ALTER TABLE webhook_secrets
  ADD COLUMN IF NOT EXISTS secret_value VARCHAR(255) NULL AFTER type_id;

CREATE INDEX IF NOT EXISTS idx_secret_value
  ON webhook_secrets (secret_value);
`)
	if err != nil {
		t.Fatalf("applyMigrationStatements: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyMigrationsDBSkipsAlreadyAppliedMigrations(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS schema_migrations")).
		WillReturnResult(driver.RowsAffected(0))

	names, err := migrations.Ordered()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM schema_migrations WHERE version=?")).
			WithArgs(name).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	}

	if err := ApplyMigrationsDB(context.Background(), db); err != nil {
		t.Fatalf("ApplyMigrationsDB: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSplitSQLStatementsKeepsSemicolonInsideString(t *testing.T) {
	stmts := splitSQLStatements(`CREATE TABLE example (value TEXT);
INSERT INTO example(value) VALUES('a;b');
-- comment with ; should not split
CREATE INDEX example_idx ON example(value);`)
	if len(stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d: %#v", len(stmts), stmts)
	}
	if stmts[1] != "INSERT INTO example(value) VALUES('a;b')" {
		t.Fatalf("unexpected second statement: %q", stmts[1])
	}
}
