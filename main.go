package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"sitecheck/db"
)

func main() {
	// Load .env from working directory; not fatal if missing — defaults apply.
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: no .env file found, using defaults (%v)\n", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("SiteCheck config:")
	fmt.Printf("  Workers:        %d\n", cfg.Workers)
	fmt.Printf("  DefaultTimeout: %ds\n", cfg.DefaultTimeout)
	fmt.Printf("  DBPath:         %s\n", cfg.DBPath)
	fmt.Printf("  ResourcesDir:   %s\n", cfg.ResourcesDir)
	fmt.Printf("  TemplatesDir:   %s\n", cfg.TemplatesDir)
	fmt.Printf("  OutputDir:      %s\n", cfg.OutputDir)
	fmt.Printf("  StaticDir:      %s\n", cfg.StaticDir)
	fmt.Printf("  SiteTitle:      %s\n", cfg.SiteTitle)
	fmt.Printf("  RetentionDays:  %d\n", cfg.RetentionDays)
	fmt.Printf("  GraphWindows:   %v\n", cfg.GraphWindows)

	// Phase 2: open DB, run migrations, verify tables.
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DB error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Migration error: %v\n", err)
		os.Exit(1)
	}

	tables, err := listTables(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Table list error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nDB tables (%d):\n", len(tables))
	for _, t := range tables {
		fmt.Printf("  - %s\n", t)
	}
}

// listTables returns all user table names in the database.
func listTables(db *db.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
