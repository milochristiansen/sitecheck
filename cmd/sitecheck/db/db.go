package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"sitecheck/core"
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
	for _, p := range core.All() {
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

	// Resource site membership, persisted so extra-site membership survives an outpost outage
	// (the core has no other source of meta() data for downed outposts).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS resource_meta (
		slug         TEXT NOT NULL,
		outpost_slug TEXT NOT NULL,
		sites_json   TEXT NOT NULL DEFAULT '{}',
		updated_at   TEXT NOT NULL,
		PRIMARY KEY (slug, outpost_slug)
	)`); err != nil {
		return fmt.Errorf("migrate resource_meta: %w", err)
	}
	return nil
}

// UpsertResourceMeta stores the site membership map for a resource, overwriting any prior entry.
// sites is serialized as JSON; nil is stored as an empty object.
func (db *DB) UpsertResourceMeta(slug, outpostSlug string, sites map[string]string) error {
	if sites == nil {
		sites = map[string]string{}
	}
	b, err := json.Marshal(sites)
	if err != nil {
		return fmt.Errorf("marshal sites for %s/%s: %w", slug, outpostSlug, err)
	}
	_, err = db.Exec(`INSERT INTO resource_meta (slug, outpost_slug, sites_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(slug, outpost_slug) DO UPDATE SET
			sites_json = excluded.sites_json,
			updated_at = excluded.updated_at`,
		slug, outpostSlug, string(b), time.Now().UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("upsert resource meta %s/%s: %w", slug, outpostSlug, err)
	}
	return nil
}

// ResourceMeta returns the stored site membership map for a resource, or (nil, false) when no
// row exists or the stored JSON is unreadable.
func (db *DB) ResourceMeta(slug, outpostSlug string) (map[string]string, bool) {
	var raw string
	err := db.QueryRow(
		`SELECT sites_json FROM resource_meta WHERE slug = ? AND outpost_slug = ?`,
		slug, outpostSlug,
	).Scan(&raw)
	if err != nil {
		return nil, false
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false
	}
	return m, true
}

// LastPass returns the pass value from the most recent real check for the given slug in the check table for checkType.
// UNKNOWN (-1) rows are skipped — they are core-injected sentinels for outpost connectivity, not real check results.
// Returns (0, false, nil) when no prior real check exists.
func (db *DB) LastPass(slug, outpostSlug, checkType string) (int, bool, error) {
	p, ok := core.ByName(checkType)
	if !ok {
		return 0, false, fmt.Errorf("unknown check type %q", checkType)
	}
	table := p.TableName()
	var pass int
	err := db.QueryRow(
		fmt.Sprintf("SELECT pass FROM %s WHERE slug = ? AND outpost_slug = ? AND pass != %d ORDER BY timestamp DESC LIMIT 1", table, core.UNKNOWN),
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
	for _, p := range core.All() {
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
	for _, p := range core.All() {
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
	for _, p := range core.All() {
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
