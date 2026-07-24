package db

import (
	"fmt"
	"time"
)

type OutpostCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	CheckCount     int
	FailCount      int
	Error          string
}

// InsertOutpostCheck inserts a row into checks_outpost and returns the new row ID.
func InsertOutpostCheck(db *DB, c OutpostCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_outpost
			(slug, outpost_slug, duration_ms, pass, response_time_ms, check_count, fail_count, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.OutpostSlug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.CheckCount, c.FailCount, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert outpost check: %w", err)
	}
	return result.LastInsertId()
}

// OutpostChecksBySlugSince returns outpost checks for a slug since the given time, oldest first.
func OutpostChecksBySlugSince(db *DB, slug, outpostSlug string, since time.Time) ([]OutpostCheck, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			check_count, fail_count, error
		FROM checks_outpost WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query outpost checks since: %w", err)
	}
	defer rows.Close()

	var checks []OutpostCheck
	for rows.Next() {
		var c OutpostCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.CheckCount, &c.FailCount, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan outpost check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
