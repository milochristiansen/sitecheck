package main

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all parsed configuration values for the outpost.
type Config struct {
	Token          string
	ResourcesDir   string
	Workers        int
	Listen         string
	DefaultTimeout int
}

// LoadConfig parses configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Token:          strEnv("SITECHECK_TOKEN", ""),
		ResourcesDir:   strEnv("SITECHECK_RESOURCES_DIR", "resources"),
		Workers:        intEnv("SITECHECK_WORKERS", 4),
		Listen:         strEnv("SITECHECK_LISTEN", ":8080"),
		DefaultTimeout: intEnv("SITECHECK_DEFAULT_TIMEOUT", 30),
	}
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.Workers < 1 {
		return fmt.Errorf("SITECHECK_WORKERS must be >= 1, got %d", c.Workers)
	}
	if c.DefaultTimeout < 1 {
		return fmt.Errorf("SITECHECK_DEFAULT_TIMEOUT must be >= 1, got %d", c.DefaultTimeout)
	}
	return nil
}

func strEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) int {
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
