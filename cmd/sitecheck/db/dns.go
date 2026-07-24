package db

import (
	"fmt"
	"time"
)

type DNSCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	Host           string
	IPs            string // JSON array of IP strings
	Error          string
}

// InsertDNSCheck inserts a row into checks_dns and returns the new row ID.
func InsertDNSCheck(db *DB, c DNSCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_dns
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, ips, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.OutpostSlug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.Host, c.IPs, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert dns check: %w", err)
	}
	return result.LastInsertId()
}


// DNSChecksBySlugSince returns DNS checks for a slug since the given time, oldest first.
func DNSChecksBySlugSince(db *DB, slug, outpostSlug string, since time.Time) ([]DNSCheck, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, ips, error
		FROM checks_dns WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`, slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query dns checks since: %w", err)
	}
	defer rows.Close()

	var checks []DNSCheck
	for rows.Next() {
		var c DNSCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.Host, &c.IPs, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dns check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
