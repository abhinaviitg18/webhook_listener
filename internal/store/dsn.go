package store

import (
	"net/url"
	"strings"

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
