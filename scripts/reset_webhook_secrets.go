package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"agenthook.store/internal/security"
)

const listenerTypePrefix = "lis::"

type webhookTypeRow struct {
	AccountID string
	Account   string
	TypeID    string
	TypeKey   string
}

func main() {
	var (
		dsn         = flag.String("dsn", "", "MySQL DSN")
		publicBase  = flag.String("public-base-url", "https://app.agenthook.store", "public base URL")
		accountSlug = flag.String("account-slug", "", "optional account slug filter")
		dryRun      = flag.Bool("dry-run", false, "show planned replacements without mutating DB")
	)
	flag.Parse()

	if strings.TrimSpace(*dsn) == "" {
		log.Fatal("--dsn is required")
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	if err := ensureSecretValueColumn(ctx, db); err != nil {
		log.Fatal(err)
	}

	rows, err := listWebhookTypes(ctx, db, strings.TrimSpace(*accountSlug))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("resetting secrets for %d webhook types\n", len(rows))
	for _, row := range rows {
		rawSecret, err := resetTypeSecrets(ctx, db, row, *dryRun)
		if err != nil {
			log.Fatalf("reset failed for account=%s type=%s: %v", row.Account, row.TypeKey, err)
		}
		fmt.Printf("%s\t%s\t%s\n", row.Account, row.TypeKey, buildWebhookURL(strings.TrimRight(*publicBase, "/"), row, rawSecret))
	}
}

func ensureSecretValueColumn(ctx context.Context, db *sql.DB) error {
	exists, err := columnExists(ctx, db, "webhook_secrets", "secret_value")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := db.ExecContext(ctx, `ALTER TABLE webhook_secrets ADD COLUMN secret_value VARCHAR(255) NULL AFTER type_id`); err != nil {
			return err
		}
	}

	indexExists, err := indexExists(ctx, db, "webhook_secrets", "idx_secret_value")
	if err != nil {
		return err
	}
	if !indexExists {
		if _, err := db.ExecContext(ctx, `CREATE INDEX idx_secret_value ON webhook_secrets (secret_value)`); err != nil {
			return err
		}
	}
	return nil
}

func listWebhookTypes(ctx context.Context, db *sql.DB, accountSlug string) ([]webhookTypeRow, error) {
	query := `
SELECT wt.account_id, a.slug, wt.id, wt.type_key
FROM webhook_types wt
JOIN accounts a ON a.id = wt.account_id
`
	args := []any{}
	if accountSlug != "" {
		query += ` WHERE a.slug = ?`
		args = append(args, accountSlug)
	}
	query += ` ORDER BY a.slug, wt.type_key`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []webhookTypeRow
	for rows.Next() {
		var item webhookTypeRow
		if err := rows.Scan(&item.AccountID, &item.Account, &item.TypeID, &item.TypeKey); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func resetTypeSecrets(ctx context.Context, db *sql.DB, row webhookTypeRow, dryRun bool) (string, error) {
	rawSecret, err := security.NewToken(18)
	if err != nil {
		return "", err
	}
	if dryRun {
		return rawSecret, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE webhook_secrets SET status='revoked' WHERE account_id=? AND type_id=? AND status='active'`, row.AccountID, row.TypeID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO webhook_secrets(id, account_id, type_id, secret_value, status, created_at) VALUES(?,?,?,?, 'active', UTC_TIMESTAMP())`, uuid.NewString(), row.AccountID, row.TypeID, rawSecret); err != nil {
		return "", err
	}

	return rawSecret, tx.Commit()
}

func buildWebhookURL(base string, row webhookTypeRow, secret string) string {
	provider, listenerID, ok := parseListenerTypeKey(row.TypeKey)
	if ok {
		return fmt.Sprintf("%s/ingest/%s/%s/%s/%s", base, row.Account, provider, listenerID, secret)
	}
	return fmt.Sprintf("%s/url/%s/%s/%s", base, row.Account, row.TypeKey, secret)
}

func parseListenerTypeKey(typeKey string) (provider string, listenerID string, ok bool) {
	raw := strings.TrimSpace(typeKey)
	if !strings.HasPrefix(raw, listenerTypePrefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(raw, listenerTypePrefix), "::")
	if len(parts) != 3 {
		return "", "", false
	}
	provider = strings.TrimSpace(parts[0])
	listenerID = strings.TrimSpace(parts[1])
	if provider == "" || listenerID == "" {
		return "", "", false
	}
	return provider, listenerID, true
}

func columnExists(ctx context.Context, db *sql.DB, tableName, columnName string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		tableName, columnName,
	).Scan(&count)
	return count > 0, err
}

func indexExists(ctx context.Context, db *sql.DB, tableName, indexName string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		tableName, indexName,
	).Scan(&count)
	return count > 0, err
}
