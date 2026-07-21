package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps *sql.DB with SiteCheck-specific operations.
type DB struct {
	*sql.DB
}

// Open opens a SQLite database at path, enabling WAL and foreign keys.
// Creates parent directories if they don't exist.
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
		return nil, fmt.Errorf("set wal: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set fk: %w", err)
	}
	return &DB{db}, nil
}

// Migrate creates all check tables and indexes if they don't exist.
func (db *DB) Migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS checks_http (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			status_code     INTEGER,
			url             TEXT NOT NULL,
			body_size       INTEGER,
			tls_version     TEXT,
			remote_ip       TEXT,
			redirect_count  INTEGER,
			error           TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_http_slug_time ON checks_http(slug, timestamp)`,

		`CREATE TABLE IF NOT EXISTS checks_ping (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			packets_sent    INTEGER,
			packets_received INTEGER,
			packet_loss_pct REAL,
			min_ms          REAL,
			max_ms          REAL,
			host            TEXT NOT NULL,
			error           TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_ping_slug_time ON checks_ping(slug, timestamp)`,

		`CREATE TABLE IF NOT EXISTS checks_tcp (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			host            TEXT NOT NULL,
			port            INTEGER NOT NULL,
			remote_ip       TEXT,
			error           TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_tcp_slug_time ON checks_tcp(slug, timestamp)`,

		`CREATE TABLE IF NOT EXISTS checks_dns (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			host            TEXT NOT NULL,
			ips             TEXT,
			error           TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_dns_slug_time ON checks_dns(slug, timestamp)`,

		`CREATE TABLE IF NOT EXISTS checks_ssl (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			host            TEXT NOT NULL,
			port            INTEGER NOT NULL DEFAULT 443,
			issuer          TEXT,
			subject         TEXT,
			not_before      TEXT,
			not_after       TEXT,
			days_remaining  INTEGER,
			error           TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_ssl_slug_time ON checks_ssl(slug, timestamp)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migrate: %w\n%s", err, m)
		}
	}
	return nil
}

// PurgeOld deletes rows older than retentionDays across all check tables.
func (db *DB) PurgeOld(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC().Format("2006-01-02 15:04:05")
	tables := []string{"checks_http", "checks_ping", "checks_tcp", "checks_dns", "checks_ssl"}
	for _, table := range tables {
		_, err := db.Exec("DELETE FROM "+table+" WHERE timestamp < ?", cutoff)
		if err != nil {
			return fmt.Errorf("purge %s: %w", table, err)
		}
	}
	return nil
}
