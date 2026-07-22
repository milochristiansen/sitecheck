package db

import (
	"fmt"
	"time"
)

// TCPCheck holds a single row from checks_tcp.
type TCPCheck struct {
	ID             int64
	Slug           string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	Host           string
	Port           int
	RemoteIP       string
	Error          string
}

// InsertTCPCheck inserts a row into checks_tcp and returns the new row ID.
func InsertTCPCheck(db *DB, c TCPCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_tcp
			(slug, duration_ms, pass, response_time_ms, host, port, remote_ip, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.Host, c.Port, c.RemoteIP, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert tcp check: %w", err)
	}
	return result.LastInsertId()
}


// TCPChecksBySlugSince returns TCP checks for a slug since the given time, oldest first.
func TCPChecksBySlugSince(db *DB, slug string, since time.Time) ([]TCPCheck, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, port, remote_ip, error
		FROM checks_tcp WHERE slug = ? AND timestamp >= ? ORDER BY timestamp`, slug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query tcp checks since: %w", err)
	}
	defer rows.Close()

	var checks []TCPCheck
	for rows.Next() {
		var c TCPCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.Host, &c.Port, &c.RemoteIP, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan tcp check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
