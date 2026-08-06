// Package envcfg provides shared environment-variable parsing for both
// binaries. Configuration structs and validation stay in each command's
// config.go; only the lookup helpers are common.
package envcfg

import (
	"fmt"
	"os"
	"strconv"
)

// Str returns the environment variable value, or fallback when the variable
// is unset or empty.
func Str(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// Int returns the environment variable parsed as an int, or fallback when the
// variable is unset or empty. A present but non-integer value panics: config
// errors should fail loudly at startup.
func Int(key string, fallback int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		panic(fmt.Sprintf("%s must be an integer, got %q", key, raw))
	}
	return v
}
