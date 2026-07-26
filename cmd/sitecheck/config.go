package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all parsed configuration values for the core.
type Config struct {
	OutpostBin     string
	OutpostWorkers int
	DefaultTimeout int
	OutpostsDir    string
	ResourcesDir   string
	DBPath         string
	TemplatesDir   string
	OutputDir      string
	StaticDir      string
	SiteTitle      string
	RetentionDays  int
	GraphWindows   []int
	NtfyServer     string
}

// LoadConfig parses configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		OutpostBin:     strEnv("SITECHECK_OUTPOST_BIN", "./scoutpost"),
		OutpostWorkers: intEnv("SITECHECK_OUTPOST_WORKERS", 4),
		DefaultTimeout: intEnv("SITECHECK_DEFAULT_TIMEOUT", 30),
		OutpostsDir:    strEnv("SITECHECK_OUTPOSTS_DIR", "outposts"),
		ResourcesDir:   strEnv("SITECHECK_RESOURCES_DIR", "resources"),
		DBPath:         strEnv("SITECHECK_DB_PATH", "data/sitecheck.db"),
		TemplatesDir:   strEnv("SITECHECK_TEMPLATES_DIR", "templates"),
		OutputDir:      strEnv("SITECHECK_OUTPUT_DIR", "output"),
		StaticDir:      strEnv("SITECHECK_STATIC_DIR", "static"),
		SiteTitle:      strEnv("SITECHECK_SITE_TITLE", "SiteCheck Status"),
		RetentionDays:  intEnv("SITECHECK_RETENTION_DAYS", 90),
		GraphWindows:   intSliceEnv("SITECHECK_GRAPH_WINDOWS", []int{24, 168, 720}),
		NtfyServer:     strEnv("SITECHECK_NTFY_SERVER", ""),
	}
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.OutpostWorkers < 1 {
		return fmt.Errorf("SITECHECK_OUTPOST_WORKERS must be >= 1, got %d", c.OutpostWorkers)
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
