package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"agenthook.store/db/migrations"
)

var (
	addColumnIfNotExistsRE   = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+` + "`?" + `([a-zA-Z0-9_]+)` + "`?" + `\s+ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+` + "`?" + `([a-zA-Z0-9_]+)` + "`?" + `\s+(.+)$`)
	createIndexIfNotExistsRE = regexp.MustCompile(`(?is)^CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+` + "`?" + `([a-zA-Z0-9_]+)` + "`?" + `\s+ON\s+` + "`?" + `([a-zA-Z0-9_]+)` + "`?" + `\s*(.+)$`)
)

// ApplyMigrations opens the configured MySQL-compatible store and applies all
// embedded schema migrations before the HTTP runtime starts accepting traffic.
func ApplyMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		return err
	}
	return ApplyMigrationsDB(ctx, db)
}

// ApplyMigrationsDB applies embedded migrations using an existing database
// handle. It is exported for tests and operational tooling.
func ApplyMigrationsDB(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		checksum VARCHAR(64) NOT NULL,
		applied_at DATETIME NOT NULL
	)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	names, err := migrations.Ordered()
	if err != nil {
		return err
	}
	for _, name := range names {
		contents, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		sumBytes := sha256.Sum256(contents)
		checksum := hex.EncodeToString(sumBytes[:])

		applied, err := migrationApplied(ctx, db, name)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		if err := applyMigrationStatements(ctx, db, string(contents)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version, checksum, applied_at) VALUES(?,?,UTC_TIMESTAMP())`, name, checksum); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		log.Printf("mysql.migration_applied version=%s", name)
	}
	return nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, version).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

type execer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func applyMigrationStatements(ctx context.Context, db execer, sqlText string) error {
	for _, stmt := range splitSQLStatements(sqlText) {
		if err := execMigrationStatement(ctx, db, stmt); err != nil {
			return err
		}
	}
	return nil
}

func execMigrationStatement(ctx context.Context, db execer, stmt string) error {
	if match := addColumnIfNotExistsRE.FindStringSubmatch(stmt); match != nil {
		tableName, columnName, columnDef := match[1], match[2], strings.TrimSpace(match[3])
		exists, err := columnExistsContext(ctx, db, tableName, columnName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", tableName, columnName, columnDef))
		return err
	}

	if match := createIndexIfNotExistsRE.FindStringSubmatch(stmt); match != nil {
		unique, indexName, tableName, indexDef := match[1], match[2], match[3], strings.TrimSpace(match[4])
		exists, err := indexExistsContext(ctx, db, tableName, indexName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE %sINDEX `%s` ON `%s` %s", unique, indexName, tableName, indexDef))
		return err
	}

	_, err := db.ExecContext(ctx, stmt)
	return err
}

func columnExistsContext(ctx context.Context, db execer, tableName, columnName string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		tableName, columnName,
	).Scan(&exists)
	return exists > 0, err
}

func indexExistsContext(ctx context.Context, db execer, tableName, indexName string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		tableName, indexName,
	).Scan(&exists)
	return exists > 0, err
}

func splitSQLStatements(sqlText string) []string {
	var stmts []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	inLineComment := false

	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]
		next := byte(0)
		if i+1 < len(sqlText) {
			next = sqlText[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				b.WriteByte(ch)
			}
			continue
		}
		if !inSingle && !inDouble && ch == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		}
		if ch == ';' && !inSingle && !inDouble {
			if stmt := strings.TrimSpace(b.String()); stmt != "" {
				stmts = append(stmts, stmt)
			}
			b.Reset()
			continue
		}
		b.WriteByte(ch)
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		stmts = append(stmts, stmt)
	}
	return stmts
}
