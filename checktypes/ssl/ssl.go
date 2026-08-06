// Package ssl implements the core.CheckPlugin interface for SSL/TLS certificate checks.
package ssl

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/core"
)

// ---------------------------------------------------------------------------
// Result struct
// ---------------------------------------------------------------------------

// SSLResult is the type-specific result returned by an SSL certificate check.
type SSLResult struct {
	Pass           int
	FailReason     string
	Host           string
	Port           int
	Issuer         string
	Subject        string
	NotBefore      string
	NotAfter       string
	DaysRemaining  int
	ResponseTimeMS float64
	Error          string
}

func (r *SSLResult) CheckType() string        { return "ssl" }
func (r *SSLResult) CheckPass() int           { return r.Pass }
func (r *SSLResult) CheckFailReason() string  { return r.FailReason }
func (r *SSLResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// ---------------------------------------------------------------------------
// DB row struct
// ---------------------------------------------------------------------------

// SSLCheck is the database row for checks_ssl.
type SSLCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
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

// ---------------------------------------------------------------------------
// Plugin implementation
// ---------------------------------------------------------------------------

type impl struct{}

func (p *impl) TypeName() string { return "ssl" }

func (p *impl) TableName() string { return "checks_ssl" }

func (p *impl) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_ssl (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			outpost_slug    TEXT NOT NULL DEFAULT '',
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			host            TEXT NOT NULL,
			port            INTEGER NOT NULL DEFAULT 443,
			issuer          TEXT,
			subject         TEXT,
			not_before      TEXT,
			not_after       TEXT,
			days_remaining  INTEGER,
			error           TEXT
		)`,
		`ALTER TABLE checks_ssl ADD COLUMN outpost_slug TEXT NOT NULL DEFAULT ''`,
	}
}

func (p *impl) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_ssl_slug_time ON checks_ssl(slug, timestamp)`,
	}
}

// Insert deserializes wire data and inserts a row.
func (p *impl) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r SSLResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal ssl data: %w", err)
	}
	_, err := db.Exec(
		`INSERT INTO checks_ssl
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, port, issuer, subject, not_before, not_after, days_remaining, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.Host, r.Port, r.Issuer, r.Subject, r.NotBefore, r.NotAfter, r.DaysRemaining, r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert ssl check: %w", err)
	}
	return nil
}

// InsertError inserts a minimal error-only row.
func (p *impl) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_ssl
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, port, error)
		VALUES (?, ?, ?, ?, 0, ?, 0, ?)`,
		slug, outpostSlug, elapsedMS, pass, "(error)", errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert ssl error check: %w", err)
	}
	return nil
}

// QuerySince returns all rows for a slug+outpost since the given time, oldest first.
func (p *impl) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, port, issuer, subject, not_before, not_after, days_remaining, error
		FROM checks_ssl WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query ssl checks since: %w", err)
	}
	defer rows.Close()

	var checks []SSLCheck
	for rows.Next() {
		var (
			c             SSLCheck
			durationMS    sql.NullInt64
			responseMS    sql.NullFloat64
			host          sql.NullString
			port          sql.NullInt64
			issuer        sql.NullString
			subject       sql.NullString
			notBefore     sql.NullString
			notAfter      sql.NullString
			daysRemaining sql.NullInt64
			errMsg        sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &host, &port, &issuer, &subject,
			&notBefore, &notAfter, &daysRemaining, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ssl check since: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Host = host.String
		c.Port = int(port.Int64)
		c.Issuer = issuer.String
		c.Subject = subject.String
		c.NotBefore = notBefore.String
		c.NotAfter = notAfter.String
		c.DaysRemaining = int(daysRemaining.Int64)
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// ExtractPoints converts a []SSLCheck to []CheckPoint for sparklines and charts.
func (p *impl) ExtractPoints(history interface{}) []core.CheckPoint {
	h, ok := history.([]SSLCheck)
	if !ok {
		return nil
	}
	pts := make([]core.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = core.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

// ExtractDurationPoints returns nil — SSL checks don't have meaningful duration data.
func (p *impl) ExtractDurationPoints(history interface{}) []core.CheckPoint {
	return nil
}

// LatestRecent splits the history slice into latest (pointer to last element)
// and recent (reversed slice of the rest, newest-first, capped at maxRecent).
func (p *impl) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]SSLCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	n := len(h) - 1
	if n > maxRecent {
		n = maxRecent
	}
	rev := make([]SSLCheck, n)
	for i := range n {
		rev[i] = h[len(h)-2-i]
	}
	return latest, rev, n
}

// ---------------------------------------------------------------------------
// Lua registration
// ---------------------------------------------------------------------------

// RegisterLua pushes the ssl_certificate function onto the Lua state.
func (p *impl) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)
		port := 443
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypNumber {
			port = int(l.ToInt(2))
		}

		timeout := defaultTimeout
		insecureSkipVerify := false
		if !l.IsNil(3) && l.TypeOf(3) == lua.TypTable {
			timeout = core.ReadIntOpt(l, 3, "timeout", defaultTimeout)
			insecureSkipVerify = core.ReadBoolOpt(l, 3, "insecure_skip_verify", false)
		}

		r := &SSLResult{
			Pass: core.FAIL,
			Host: host,
			Port: port,
		}

		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		dialer := &net.Dialer{Timeout: time.Duration(timeout) * time.Second}

		start := time.Now()
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		})
		elapsed := time.Since(start)
		r.ResponseTimeMS = elapsed.Seconds() * 1000

		if err != nil {
			r.Error = err.Error()
			pushSSLResult(l, r)
			return 1
		}
		defer conn.Close()

		certs := conn.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			r.Error = "no certificates presented"
			pushSSLResult(l, r)
			return 1
		}

		cert := certs[0]
		r.Subject = cert.Subject.String()
		r.Issuer = cert.Issuer.String()
		r.NotBefore = cert.NotBefore.Format(time.RFC3339)
		r.NotAfter = cert.NotAfter.Format(time.RFC3339)
		r.DaysRemaining = int(time.Until(cert.NotAfter).Hours() / 24)

		pushSSLResult(l, r)
		return 1
	})
	l.SetGlobal("ssl_certificate")
}

func pushSSLResult(l *lua.State, r *SSLResult) {
	core.PushResultUserData(l, r, map[string]core.LuaField{
		"Pass":           {Get: func(l *lua.State) { l.Push(int64(r.Pass)) }, Set: func(l *lua.State) { r.Pass = int(l.ToInt(3)) }},
		"FailReason":     {Get: func(l *lua.State) { l.Push(r.FailReason) }, Set: func(l *lua.State) { r.FailReason = l.ToString(3) }},
		"Host":           {Get: func(l *lua.State) { l.Push(r.Host) }, Set: func(l *lua.State) { r.Host = l.ToString(3) }},
		"Port":           {Get: func(l *lua.State) { l.Push(int64(r.Port)) }, Set: func(l *lua.State) { r.Port = int(l.ToInt(3)) }},
		"Issuer":         {Get: func(l *lua.State) { l.Push(r.Issuer) }, Set: func(l *lua.State) { r.Issuer = l.ToString(3) }},
		"Subject":        {Get: func(l *lua.State) { l.Push(r.Subject) }, Set: func(l *lua.State) { r.Subject = l.ToString(3) }},
		"NotBefore":      {Get: func(l *lua.State) { l.Push(r.NotBefore) }, Set: func(l *lua.State) { r.NotBefore = l.ToString(3) }},
		"NotAfter":       {Get: func(l *lua.State) { l.Push(r.NotAfter) }, Set: func(l *lua.State) { r.NotAfter = l.ToString(3) }},
		"DaysRemaining":  {Get: func(l *lua.State) { l.Push(int64(r.DaysRemaining)) }},
		"ResponseTimeMS": {Get: func(l *lua.State) { l.Push(r.ResponseTimeMS) }},
		"Error":          {Get: func(l *lua.State) { l.Push(r.Error) }, Set: func(l *lua.State) { r.Error = l.ToString(3) }},
	})
}

// ---------------------------------------------------------------------------
// Wire dispatch
// ---------------------------------------------------------------------------

// DispatchWireResult converts the CheckResult returned by a Lua check() call
// into a core.WireResult.
func (p *impl) DispatchWireResult(res core.ResourceMeta, cr core.CheckResult, elapsed time.Duration) core.WireResult {
	r := cr.(*SSLResult)
	return core.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"ssl", r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error,
		r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

func (p *impl) TemplateNames() (row, body string) {
	return "check_ssl_row", "check_ssl_body"
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func init() {
	core.Register(&impl{})
}
