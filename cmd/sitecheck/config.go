package main

import (
	"fmt"

	"sitecheck/envcfg"
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
		OutpostBin:      envcfg.Str("SITECHECK_OUTPOST_BIN", "./scoutpost"),
		OutpostWorkers:  envcfg.Int("SITECHECK_OUTPOST_WORKERS", 4),
		DefaultTimeout:  envcfg.Int("SITECHECK_DEFAULT_TIMEOUT", 30),
		OutpostsDir:     envcfg.Str("SITECHECK_OUTPOSTS_DIR", "outposts"),
		ResourcesDir:    envcfg.Str("SITECHECK_RESOURCES_DIR", "resources"),
		DBPath:          envcfg.Str("SITECHECK_DB_PATH", "data/sitecheck.db"),
		TemplatesDir:    envcfg.Str("SITECHECK_TEMPLATES_DIR", "templates"),
		OutputDir:       envcfg.Str("SITECHECK_OUTPUT_DIR", "output"),
		StaticDir:       envcfg.Str("SITECHECK_STATIC_DIR", "static"),
		SiteTitle:       envcfg.Str("SITECHECK_SITE_TITLE", "SiteCheck Status"),
		RetentionDays:   envcfg.Int("SITECHECK_RETENTION_DAYS", 90),
		NtfyServer:      envcfg.Str("SITECHECK_NTFY_SERVER", ""),
		TelegramToken:   envcfg.Str("SITECHECK_TELEGRAM_TOKEN", ""),
		TelegramChannel: envcfg.Str("SITECHECK_TELEGRAM_CHANNEL", ""),
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
