package db

import "fmt"

// DNSCheck holds a single row from checks_dns.
type DNSCheck struct {
	ID             int64
	Slug           string
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
			(slug, duration_ms, pass, response_time_ms, host, ips, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.Host, c.IPs, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert dns check: %w", err)
	}
	return result.LastInsertId()
}

// DNSChecksBySlug returns all DNS checks for a slug, newest first.
func DNSChecksBySlug(db *DB, slug string) ([]DNSCheck, error) {
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, ips, error
		FROM checks_dns WHERE slug = ? ORDER BY timestamp DESC`, slug,
	)
	if err != nil {
		return nil, fmt.Errorf("query dns checks: %w", err)
	}
	defer rows.Close()

	var checks []DNSCheck
	for rows.Next() {
		var c DNSCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.Host, &c.IPs, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dns check: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
