// Package dns implements core.CheckPlugin for DNS resolution checks.
package dns

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/core"
)

// DNSResult is the result of a DNS lookup check. It implements core.CheckResult.
type DNSResult struct {
	Pass           int
	FailReason     string
	Host           string
	IPs            string
	ResponseTimeMS float64
	Error          string
}

func (r *DNSResult) CheckType() string        { return "dns" }
func (r *DNSResult) CheckPass() int           { return r.Pass }
func (r *DNSResult) CheckFailReason() string  { return r.FailReason }
func (r *DNSResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// DNSCheck is a database row from checks_dns.
type DNSCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	Host           string
	IPs            string
	Error          string
}

type impl struct{}

func (impl) TypeName() string { return "dns" }

func (impl) TableName() string { return "checks_dns" }

func (impl) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_dns (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			outpost_slug    TEXT NOT NULL DEFAULT '',
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			host            TEXT NOT NULL,
			ips    TEXT,
			error           TEXT
		)`,
		`ALTER TABLE checks_dns ADD COLUMN outpost_slug TEXT NOT NULL DEFAULT ''`,
	}
}

func (impl) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_dns_slug_time ON checks_dns(slug, timestamp)`,
	}
}

func (impl) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r DNSResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal dns data: %w", err)
	}
	_, err := db.Exec(
		`INSERT INTO checks_dns
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, ips, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.Host, r.IPs, r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert dns check: %w", err)
	}
	return nil
}

func (impl) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_dns
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, ips, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, 0.0, "(error)", "", errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert dns check error: %w", err)
	}
	return nil
}

func (impl) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, ips, error
		FROM checks_dns WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query dns checks since: %w", err)
	}
	defer rows.Close()

	var checks []DNSCheck
	for rows.Next() {
		var (
			c          DNSCheck
			durationMS sql.NullInt64
			responseMS sql.NullFloat64
			host       sql.NullString
			resolvedIP sql.NullString
			errMsg     sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &host, &resolvedIP, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dns check since: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Host = host.String
		c.IPs = resolvedIP.String
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows dns check since: %w", err)
	}
	return checks, nil
}

func (impl) ExtractPoints(history interface{}) []core.CheckPoint {
	h, ok := history.([]DNSCheck)
	if !ok {
		return nil
	}
	pts := make([]core.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = core.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (impl) ExtractDurationPoints(history interface{}) []core.CheckPoint {
	return nil
}

func (impl) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]DNSCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	if len(h) == 1 {
		return latest, nil, 0
	}
	n := len(h) - 1
	if n > maxRecent {
		n = maxRecent
	}
	reversed := make([]DNSCheck, n)
	for i := range n {
		reversed[i] = h[len(h)-2-i]
	}
	return latest, reversed, n
}

func (impl) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)

		timeout := defaultTimeout
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			timeout = core.ReadIntOpt(l, 2, "timeout", defaultTimeout)
		}

		r := &DNSResult{
			Pass: core.FAIL,
			Host: host,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		start := time.Now()
		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		elapsed := time.Since(start)
		r.ResponseTimeMS = elapsed.Seconds() * 1000

		if err != nil {
			r.Error = err.Error()
			pushDNSResult(l, r)
			return 1
		}

		// Join resolved IPs into a comma-separated string for storage.
		for i, ip := range addrs {
			if i > 0 {
				r.IPs += ", "
			}
			r.IPs += ip
		}

		pushDNSResult(l, r)
		return 1
	})
	l.SetGlobal("dns_lookup")
}

func pushDNSResult(l *lua.State, r *DNSResult) {
	core.PushResultUserData(l, r, map[string]core.LuaField{
		"Pass":           {Get: func(l *lua.State) { l.Push(int64(r.Pass)) }, Set: func(l *lua.State) { r.Pass = int(l.ToInt(3)) }},
		"FailReason":     {Get: func(l *lua.State) { l.Push(r.FailReason) }, Set: func(l *lua.State) { r.FailReason = l.ToString(3) }},
		"Host":           {Get: func(l *lua.State) { l.Push(r.Host) }, Set: func(l *lua.State) { r.Host = l.ToString(3) }},
		"IPs":            {Get: func(l *lua.State) { l.Push(r.IPs) }, Set: func(l *lua.State) { r.IPs = l.ToString(3) }},
		"ResponseTimeMS": {Get: func(l *lua.State) { l.Push(r.ResponseTimeMS) }},
		"Error":          {Get: func(l *lua.State) { l.Push(r.Error) }, Set: func(l *lua.State) { r.Error = l.ToString(3) }},
	})
}

func (impl) DispatchWireResult(res core.ResourceMeta, cr core.CheckResult, elapsed time.Duration) core.WireResult {
	r := cr.(*DNSResult)
	return core.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"dns",
		r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error,
		r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

func (impl) TemplateNames() (string, string) {
	return "check_dns_row", "check_dns_body"
}

func init() {
	core.Register(impl{})
}
