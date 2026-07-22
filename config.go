package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all parsed configuration values.
type Config struct {
	Workers        int
	DefaultTimeout int
	DBPath         string
	ResourcesDir   string
	TemplatesDir   string
	OutputDir      string
	StaticDir      string
	SiteTitle      string
	RetentionDays  int
	GraphWindows   []int
	NtfyServer     string
}

// LoadConfig loads .env via godotenv then parses and validates all keys.
// Returns the parsed config or an error if any required key is missing or invalid.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	cfg.Workers = intEnv("SITECHECK_WORKERS", 4)
	cfg.DefaultTimeout = intEnv("SITECHECK_DEFAULT_TIMEOUT", 30)
	cfg.DBPath = strEnv("SITECHECK_DB_PATH", "data/sitecheck.db")
	cfg.ResourcesDir = strEnv("SITECHECK_RESOURCES_DIR", "resources")
	cfg.TemplatesDir = strEnv("SITECHECK_TEMPLATES_DIR", "templates")
	cfg.OutputDir = strEnv("SITECHECK_OUTPUT_DIR", "output")
	cfg.StaticDir = strEnv("SITECHECK_STATIC_DIR", "static")
	cfg.SiteTitle = strEnv("SITECHECK_SITE_TITLE", "SiteCheck Status")
	cfg.RetentionDays = intEnv("SITECHECK_RETENTION_DAYS", 90)
	cfg.GraphWindows = intSliceEnv("SITECHECK_GRAPH_WINDOWS", []int{24, 168, 720})
	cfg.NtfyServer = strEnv("SITECHECK_NTFY_SERVER", "SITECHECK_NTFY_SERVER=https://ntfy.sh")

	return cfg, cfg.validate()
}

// validate checks logical constraints on the parsed config.
func (c *Config) validate() error {
	if c.Workers < 1 {
		return fmt.Errorf("SITECHECK_WORKERS must be >= 1, got %d", c.Workers)
	}
	if c.DefaultTimeout < 1 {
		return fmt.Errorf("SITECHECK_DEFAULT_TIMEOUT must be >= 1, got %d", c.DefaultTimeout)
	}
	if c.RetentionDays < 1 {
		return fmt.Errorf("SITECHECK_RETENTION_DAYS must be >= 1, got %d", c.RetentionDays)
	}
	if len(c.GraphWindows) == 0 {
		return fmt.Errorf("SITECHECK_GRAPH_WINDOWS must have at least one value")
	}
	for _, w := range c.GraphWindows {
		if w < 1 {
			return fmt.Errorf("SITECHECK_GRAPH_WINDOWS values must be >= 1, got %d", w)
		}
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

func intSliceEnv(key string, fallback []int) []int {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			panic(fmt.Sprintf("%s must be comma-separated integers, got %q", key, raw))
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
