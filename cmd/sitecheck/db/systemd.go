package db

import (
	"fmt"
	"time"
)

type SystemdCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	ServiceName    string
	ActiveState    string
	SubState       string
	LoadState      string
	MainPID        int
	Error          string
}

// InsertSystemdCheck inserts a row into checks_systemd and returns the new row ID.
func InsertSystemdCheck(db *DB, c SystemdCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_systemd
			(slug, outpost_slug, duration_ms, pass, response_time_ms, service_name, active_state, sub_state, load_state, main_pid, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.OutpostSlug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.ServiceName, c.ActiveState, c.SubState, c.LoadState, c.MainPID, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert systemd check: %w", err)
	}
	return result.LastInsertId()
}

// SystemdChecksBySlugSince returns systemd checks for a slug since the given time, oldest first.
func SystemdChecksBySlugSince(db *DB, slug, outpostSlug string, since time.Time) ([]SystemdCheck, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			service_name, active_state, sub_state, load_state, main_pid, error
		FROM checks_systemd WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`, slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query systemd checks since: %w", err)
	}
	defer rows.Close()

	var checks []SystemdCheck
	for rows.Next() {
		var c SystemdCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.ServiceName, &c.ActiveState, &c.SubState,
			&c.LoadState, &c.MainPID, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan systemd check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
