package db

import "fmt"

// PingCheck holds a single row from checks_ping.
type PingCheck struct {
	ID              int64
	Slug            string
	Timestamp       string
	DurationMS      int64
	Pass            int
	ResponseTimeMS  float64
	PacketsSent     int
	PacketsReceived int
	PacketLossPct   float64
	MinMS           float64
	MaxMS           float64
	Host            string
	Error           string
}

// InsertPingCheck inserts a row into checks_ping and returns the new row ID.
func InsertPingCheck(db *DB, c PingCheck) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO checks_ping
			(slug, duration_ms, pass, response_time_ms, packets_sent, packets_received, packet_loss_pct, min_ms, max_ms, host, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.PacketsSent, c.PacketsReceived, c.PacketLossPct, c.MinMS, c.MaxMS,
		c.Host, c.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert ping check: %w", err)
	}
	return result.LastInsertId()
}

// PingChecksBySlug returns all ping checks for a slug, newest first.
func PingChecksBySlug(db *DB, slug string) ([]PingCheck, error) {
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			packets_sent, packets_received, packet_loss_pct, min_ms, max_ms, host, error
		FROM checks_ping WHERE slug = ? ORDER BY timestamp DESC`, slug,
	)
	if err != nil {
		return nil, fmt.Errorf("query ping checks: %w", err)
	}
	defer rows.Close()

	var checks []PingCheck
	for rows.Next() {
		var c PingCheck
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &c.DurationMS, &c.Pass,
			&c.ResponseTimeMS, &c.PacketsSent, &c.PacketsReceived, &c.PacketLossPct,
			&c.MinMS, &c.MaxMS, &c.Host, &c.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ping check: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}
