package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// NormalizeMySQLDSN accepts both go-sql-driver/mysql DSNs and Railway-style
// mysql:// URLs. Existing DSNs are returned unchanged for AWS/TiDB compatibility.
func NormalizeMySQLDSN(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if !strings.HasPrefix(strings.ToLower(trimmed), "mysql://") {
		return dsn
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return dsn
	}

	password, _ := parsed.User.Password()
	cfg := mysql.NewConfig()
	cfg.User = parsed.User.Username()
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = parsed.Host
	cfg.DBName = strings.TrimPrefix(parsed.Path, "/")
	cfg.ParseTime = true

	if parsed.RawQuery != "" {
		cfg.Params = map[string]string{}
		for key, values := range parsed.Query() {
			if len(values) > 0 {
				cfg.Params[key] = values[len(values)-1]
			}
		}
	}
	return cfg.FormatDSN()
}

// EnsureMySQLDatabase creates the database named in the DSN before migrations
// run. This is primarily for Railway's one-field template path, where the MySQL
// container is created with only a root password and AgentHook owns schema init.
func EnsureMySQLDatabase(ctx context.Context, dsn string) error {
	cfg, err := mysql.ParseDSN(NormalizeMySQLDSN(dsn))
	if err != nil {
		return err
	}
	dbName := strings.TrimSpace(cfg.DBName)
	if dbName == "" {
		return nil
	}

	cfg.DBName = ""
	bootstrapDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer bootstrapDB.Close()

	bootstrapDB.SetMaxOpenConns(1)
	bootstrapDB.SetMaxIdleConns(1)
	bootstrapDB.SetConnMaxLifetime(5 * time.Minute)
	if err := bootstrapDB.PingContext(ctx); err != nil {
		return err
	}
	_, err = bootstrapDB.ExecContext(ctx, fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		strings.ReplaceAll(dbName, "`", "``"),
	))
	return err
}
