package config

import (
	"fmt"
	"os"
	"strings"
)

func EnvironMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func Value(env map[string]string, key, fallback string) string {
	if value, ok := env[key]; ok && value != "" {
		return value
	}
	return fallback
}

func LoadFile(path string, env map[string]string, allowedKeys map[string]bool) error {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid release config line in %s: %s", path, line)
		}
		if !allowedKeys[key] {
			return fmt.Errorf("unsupported release config key in %s: %s", path, key)
		}
		if _, exists := env[key]; exists {
			continue
		}
		env[key] = stripSimpleQuotes(value)
	}
	return nil
}

func stripSimpleQuotes(value string) string {
	value = strings.TrimSuffix(value, `"`)
	value = strings.TrimPrefix(value, `"`)
	value = strings.TrimSuffix(value, `'`)
	value = strings.TrimPrefix(value, `'`)
	return value
}
