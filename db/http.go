package db

import (
	"fmt"
	"time"
)

// HTTPCheck holds a single row from checks_http.
type HTTPCheck struct {
	ID             int64
	Slug           string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	StatusCode     int
	URL            string
	Body           *string
	BodySize       int64
	TLSVersion     string
	RemoteIP       string
	RedirectCount  int
	Error          string
}

// InsertHTTPCheck inserts a row into checks_http and returns the new row ID.
func InsertHTTPCheck(db *DB, c HTTPCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_http
			(slug, duration_ms, pass, response_time_ms, status_code, url, body, body_size, tls_version, remote_ip, redirect_count, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.DurationMS, c.Pass, c.ResponseTimeMS, c.StatusCode, c.URL,
		c.Body, c.BodySize, c.TLSVersion, c.RemoteIP, c.RedirectCount, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert http check: %w", err)
	}
	return result.LastInsertId()
}

// HTTPChecksBySlug returns all HTTP checks for a slug, newest first.
func HTTPChecksBySlug(db *DB, slug string) ([]HTTPCheck, error) {
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			status_code, url, body, body_size, tls_version, remote_ip, redirect_count, error
		FROM checks_http WHERE slug = ? ORDER BY timestamp DESC`, slug,
	)
	if err != nil {
		return nil, fmt.Errorf("query http checks: %w", err)
	}
	defer rows.Close()

	var checks []HTTPCheck
	for rows.Next() {
		var c HTTPCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.StatusCode, &c.URL, &c.Body, &c.BodySize,
			&c.TLSVersion, &c.RemoteIP, &c.RedirectCount, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan http check: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// HTTPChecksBySlugSince returns HTTP checks for a slug since the given time, oldest first.
func HTTPChecksBySlugSince(db *DB, slug string, since time.Time) ([]HTTPCheck, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			status_code, url, body, body_size, tls_version, remote_ip, redirect_count, error
		FROM checks_http WHERE slug = ? AND timestamp >= ? ORDER BY timestamp`, slug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query http checks since: %w", err)
	}
	defer rows.Close()

	var checks []HTTPCheck
	for rows.Next() {
		var c HTTPCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.StatusCode, &c.URL, &c.Body, &c.BodySize,
			&c.TLSVersion, &c.RemoteIP, &c.RedirectCount, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan http check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
