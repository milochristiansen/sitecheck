package db

import (
	"fmt"
	"time"
)

// SSLCheck holds a single row from checks_ssl.
type SSLCheck struct {
	ID             int64
	Slug           string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	Host           string
	Port           int
	Issuer         string
	Subject        string
	NotBefore      string
	NotAfter       string
	DaysRemaining  int
	Error          string
}

// InsertSSLCheck inserts a row into checks_ssl and returns the new row ID.
func InsertSSLCheck(db *DB, c SSLCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_ssl
			(slug, duration_ms, pass, response_time_ms, host, port, issuer, subject, not_before, not_after, days_remaining, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.Host, c.Port, c.Issuer, c.Subject, c.NotBefore, c.NotAfter, c.DaysRemaining, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert ssl check: %w", err)
	}
	return result.LastInsertId()
}


// SSLChecksBySlugSince returns SSL checks for a slug since the given time, oldest first.
func SSLChecksBySlugSince(db *DB, slug string, since time.Time) ([]SSLCheck, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, port, issuer, subject, not_before, not_after, days_remaining, error
		FROM checks_ssl WHERE slug = ? AND timestamp >= ? ORDER BY timestamp`, slug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query ssl checks since: %w", err)
	}
	defer rows.Close()

	var checks []SSLCheck
	for rows.Next() {
		var c SSLCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.Host, &c.Port, &c.Issuer, &c.Subject,
			&c.NotBefore, &c.NotAfter, &c.DaysRemaining, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ssl check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
