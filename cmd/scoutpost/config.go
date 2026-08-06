package main

import (
	"fmt"

	"sitecheck/envcfg"
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
		Token:          envcfg.Str("SITECHECK_TOKEN", ""),
		ResourcesDir:   envcfg.Str("SITECHECK_RESOURCES_DIR", "resources"),
		Workers:        envcfg.Int("SITECHECK_WORKERS", 4),
		Listen:         envcfg.Str("SITECHECK_LISTEN", ":8080"),
		DefaultTimeout: envcfg.Int("SITECHECK_DEFAULT_TIMEOUT", 30),
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
