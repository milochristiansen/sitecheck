// Package outpost implements registry.CheckPlugin for outpost health checks.
// OutpostResult is synthesized by the pool in the core binary, not by Lua scripts,
// so RegisterLua and DispatchWireResult are no-ops.
package outpost

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/milochristiansen/lua"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// OutpostResult is the wire deserialization of an outpost health result.
// It is synthesized by the core (pool.go), never produced by a Lua script.
type OutpostResult struct {
	Pass           int     `json:"pass"`
	FailReason     string  `json:"fail_reason,omitempty"`
	ResponseTimeMS float64 `json:"response_ms"`
	Error          string  `json:"error,omitempty"`
	CheckCount     int     `json:"check_count"`
	FailCount      int     `json:"fail_count"`
}

// CheckResult interface.
func (r *OutpostResult) CheckType() string       { return "outpost" }
func (r *OutpostResult) CheckPass() int           { return r.Pass }
func (r *OutpostResult) CheckFailReason() string  { return r.FailReason }
func (r *OutpostResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// OutpostCheck is the DB row model for checks_outpost.
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

type impl struct{}

func (p *impl) TypeName() string { return "outpost" }

func (p *impl) TableName() string { return "checks_outpost" }

func (p *impl) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_outpost (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			outpost_slug    TEXT NOT NULL DEFAULT '',
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			check_count     INTEGER,
			fail_count      INTEGER,
			error           TEXT
		)`,
	}
}

func (p *impl) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_outpost_slug_time ON checks_outpost(slug, timestamp)`,
	}
}

// Insert unmarshals data into OutpostResult and inserts a row into checks_outpost.
func (p *impl) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r OutpostResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal outpost data: %w", err)
	}
	_, err := db.Exec(
		`INSERT INTO checks_outpost
			(slug, outpost_slug, duration_ms, pass, response_time_ms, check_count, fail_count, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS, r.CheckCount, r.FailCount, r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert outpost check: %w", err)
	}
	return nil
}

// InsertError inserts a minimal row for an outpost that produced an error.
func (p *impl) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_outpost
			(slug, outpost_slug, duration_ms, pass, error)
		VALUES (?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert outpost check error: %w", err)
	}
	return nil
}

// QuerySince returns all outpost checks for the given slug+outpostSlug since the given time, oldest first.
func (p *impl) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
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
		var (
			c               OutpostCheck
			durationMS      sql.NullInt64
			responseTimeMS  sql.NullFloat64
			checkCount      sql.NullInt64
			failCount       sql.NullInt64
			errMsg          sql.NullString
		)
		if err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseTimeMS, &checkCount, &failCount, &errMsg,
		); err != nil {
			return nil, fmt.Errorf("scan outpost check since: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseTimeMS.Float64
		c.CheckCount = int(checkCount.Int64)
		c.FailCount = int(failCount.Int64)
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// ExtractPoints converts an []OutpostCheck history slice to CheckPoints,
// using ResponseTimeMS as the response value.
func (p *impl) ExtractPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]OutpostCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i := range h {
		pts[i] = registry.CheckPoint{Pass: h[i].Pass, Resp: h[i].ResponseTimeMS, TS: h[i].Timestamp}
	}
	return pts
}

// ExtractDurationPoints converts an []OutpostCheck history slice to CheckPoints,
// using DurationMS as the response value.
func (p *impl) ExtractDurationPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]OutpostCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i := range h {
		pts[i] = registry.CheckPoint{Pass: h[i].Pass, Resp: float64(h[i].DurationMS), TS: h[i].Timestamp}
	}
	return pts
}

// LatestRecent returns the latest check and up to maxRecent preceding checks
// in newest-first order.
func (p *impl) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]OutpostCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}

	// Latest is a pointer to the last (newest) element.
	latest = &h[len(h)-1]

	if len(h) < 2 {
		return latest, nil, 0
	}

	// Recent checks: start from the second-newest, go backward, cap at maxRecent.
	n := len(h) - 1
	if n > maxRecent {
		n = maxRecent
	}
	rc := make([]OutpostCheck, n)
	for i := range n {
		rc[i] = h[len(h)-2-i]
	}
	return latest, rc, n
}

// RegisterLua is a no-op — OutpostResult is not produced by Lua scripts.
func (p *impl) RegisterLua(l *lua.State, defaultTimeout int) {}

// DispatchWireResult is a no-op — OutpostResult is synthesized by the core,
// not produced by Lua scripts. Returns an empty WireResult with an error message.
func (p *impl) DispatchWireResult(res registry.ResourceMeta, cr protocol.CheckResult, elapsed time.Duration) protocol.WireResult {
	return protocol.NewWireResult(
		res.Slug, res.Name, res.Desc, "outpost",
		0, "", 0, elapsed.Milliseconds(),
		"outpost results are synthesized by the core",
		nil,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

func (p *impl) TemplateNames() (row, body string) {
	return "check_outpost_row", "check_outpost_body"
}

func (p *impl) TemplateFiles() []string {
	return []string{"templates/checks/outpost.html"}
}

func init() {
	registry.Register(&impl{})
}
