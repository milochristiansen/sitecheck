package db

import "fmt"

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

// TCPChecksBySlug returns all TCP checks for a slug, newest first.
func TCPChecksBySlug(db *DB, slug string) ([]TCPCheck, error) {
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, port, remote_ip, error
		FROM checks_tcp WHERE slug = ? ORDER BY timestamp DESC`, slug,
	)
	if err != nil {
		return nil, fmt.Errorf("query tcp checks: %w", err)
	}
	defer rows.Close()

	var checks []TCPCheck
	for rows.Next() {
		var c TCPCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.Host, &c.Port, &c.RemoteIP, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan tcp check: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
