package migrations

import (
	"embed"
	"io/fs"
	"sort"
)

// Files embeds the ordered SQL migrations used to bootstrap persistent stores.
//
//go:embed *.sql
var Files embed.FS

// Ordered returns migration filenames in lexical order.
func Ordered() ([]string, error) {
	entries, err := fs.ReadDir(Files, ".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) >= 4 && name[len(name)-4:] == ".sql" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
