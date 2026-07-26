package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"sitecheck/checktypes/registry"
)

// DB wraps *sql.DB with SiteCheck-specific operations.
type DB struct {
	*sql.DB
}

// Open opens a SQLite database at path, enabling WAL and foreign keys. Creates parent directories if they don't exist.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	return &DB{db}, nil
}

// Migrate creates all check tables and indexes if they don't exist.
func (db *DB) Migrate() error {
	for _, p := range registry.All() {
		for _, ddl := range p.CreateTableDDL() {
			if _, err := db.Exec(ddl); err != nil {
				if strings.Contains(err.Error(), "duplicate column") {
					continue
				}
				return fmt.Errorf("migrate %s: %w\n%s", p.TypeName(), err, ddl)
			}
		}
		for _, ddl := range p.CreateIndexDDL() {
			if _, err := db.Exec(ddl); err != nil {
				return fmt.Errorf("migrate index %s: %w\n%s", p.TypeName(), err, ddl)
			}
		}
	}
	return nil
}

// LastPass returns the pass value from the most recent real check for the given slug in the check table for checkType.
// UNKNOWN (-1) rows are skipped — they are core-injected sentinels for outpost connectivity, not real check results.
// Returns (0, false, nil) when no prior real check exists.
func (db *DB) LastPass(slug, outpostSlug, checkType string) (int, bool, error) {
	p, ok := registry.ByName(checkType)
	if !ok {
		return 0, false, fmt.Errorf("unknown check type %q", checkType)
	}
	table := p.TableName()
	var pass int
	err := db.QueryRow(
		`SELECT pass FROM `+table+` WHERE slug = ? AND outpost_slug = ? AND pass != -1 ORDER BY timestamp DESC LIMIT 1`,
		slug, outpostSlug,
	).Scan(&pass)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("last pass for %s/%s: %w", slug, checkType, err)
	}
	return pass, true, nil
}

// PurgeOld deletes rows older than retentionDays across all check tables.
func (db *DB) PurgeOld(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC().Format("2006-01-02 15:04:05")
	for _, p := range registry.All() {
		_, err := db.Exec("DELETE FROM "+p.TableName()+" WHERE timestamp < ?", cutoff)
		if err != nil {
			return fmt.Errorf("purge %s: %w", p.TableName(), err)
		}
	}
	return nil
}

// DistinctSlugsByOutpost returns all distinct (slug, check_type) pairs across all check tables that have rows for the
// given outpost_slug.
func (db *DB) DistinctSlugsByOutpost(outpostSlug string) ([]SlugType, error) {
	type pair struct {
		slug string
		typ  string
	}
	seen := map[pair]bool{}
	for _, p := range registry.All() {
		rows, err := db.Query(
			"SELECT DISTINCT slug FROM "+p.TableName()+" WHERE outpost_slug = ?",
			outpostSlug,
		)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", p.TableName(), err)
		}
		for rows.Next() {
			var slug string
			if err := rows.Scan(&slug); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s: %w", p.TableName(), err)
			}
			seen[pair{slug, p.TypeName()}] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows %s: %w", p.TableName(), err)
		}
	}
	var result []SlugType
	for p := range seen {
		result = append(result, SlugType{Slug: p.slug, Type: p.typ})
	}
	return result, nil
}

// LookupCheckType returns the check type for a given slug+outpostSlug by scanning all check tables.
// Returns ("", false) if no prior history exists.
func (db *DB) LookupCheckType(slug, outpostSlug string) (string, bool) {
	for _, p := range registry.All() {
		var count int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM "+p.TableName()+" WHERE slug = ? AND outpost_slug = ?",
			slug, outpostSlug,
		).Scan(&count); err != nil {
			continue
		}
		if count > 0 {
			return p.TypeName(), true
		}
	}
	return "", false
}

// SlugType is a slug / check-type pair returned by DistinctSlugsByOutpost.
type SlugType struct {
	Slug string
	Type string
}
