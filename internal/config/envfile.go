package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFiles hydrates process env from local.env/.env style files without
// overriding variables that are already explicitly set.
func LoadEnvFiles(paths ...string) error {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := loadEnvFile(path); err != nil {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid env line in %s: %s", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"")
		if key == "" {
			return fmt.Errorf("invalid env key in %s: %s", path, line)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from %s: %w", key, path, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}
