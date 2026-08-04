package main

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all parsed configuration values for the core.
type Config struct {
	OutpostBin      string
	OutpostWorkers  int
	DefaultTimeout  int
	OutpostsDir     string
	ResourcesDir    string
	DBPath          string
	TemplatesDir    string
	OutputDir       string
	StaticDir       string
	SiteTitle       string
	RetentionDays   int
	NtfyServer      string
	TelegramToken   string
	TelegramChannel string
}

// LoadConfig parses configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		OutpostBin:      strEnv("SITECHECK_OUTPOST_BIN", "./scoutpost"),
		OutpostWorkers:  intEnv("SITECHECK_OUTPOST_WORKERS", 4),
		DefaultTimeout:  intEnv("SITECHECK_DEFAULT_TIMEOUT", 30),
		OutpostsDir:     strEnv("SITECHECK_OUTPOSTS_DIR", "outposts"),
		ResourcesDir:    strEnv("SITECHECK_RESOURCES_DIR", "resources"),
		DBPath:          strEnv("SITECHECK_DB_PATH", "data/sitecheck.db"),
		TemplatesDir:    strEnv("SITECHECK_TEMPLATES_DIR", "templates"),
		OutputDir:       strEnv("SITECHECK_OUTPUT_DIR", "output"),
		StaticDir:       strEnv("SITECHECK_STATIC_DIR", "static"),
		SiteTitle:       strEnv("SITECHECK_SITE_TITLE", "SiteCheck Status"),
		RetentionDays:   intEnv("SITECHECK_RETENTION_DAYS", 90),
		NtfyServer:      strEnv("SITECHECK_NTFY_SERVER", ""),
		TelegramToken:   strEnv("SITECHECK_TELEGRAM_TOKEN", ""),
		TelegramChannel: strEnv("SITECHECK_TELEGRAM_CHANNEL", ""),
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
