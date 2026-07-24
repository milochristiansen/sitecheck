// Package dns implements the registry.CheckPlugin interface for DNS lookup checks.
package dns

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// DNSResult is the check-type-specific result for a DNS lookup.
type DNSResult struct {
	Pass           int
	FailReason     string
	Host           string
	IPs            []string
	ResponseTimeMS float64
	Error          string
}

func (r *DNSResult) CheckType() string       { return "dns" }
func (r *DNSResult) CheckPass() int          { return r.Pass }
func (r *DNSResult) CheckFailReason() string { return r.FailReason }
func (r *DNSResult) CheckResponseMS() float64  { return r.ResponseTimeMS }

// DNSCheck is the database row for a DNS check.
type DNSCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	Host           string
	IPs            string // JSON array of IP strings
	Error          string
}

type plugin struct{}

func (p *plugin) TypeName() string { return "dns" }

func (p *plugin) TableName() string { return "checks_dns" }

func (p *plugin) CreateTableDDL() []string {
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
			ips             TEXT,
			error           TEXT
		)`,
	}
}

func (p *plugin) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_dns_slug_time ON checks_dns(slug, timestamp)`,
	}
}

func (p *plugin) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r DNSResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal dns result: %w", err)
	}

	ipsJSON, err := json.Marshal(r.IPs)
	if err != nil {
		return fmt.Errorf("marshal dns ips: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO checks_dns
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, ips, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.Host, string(ipsJSON), r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert dns check: %w", err)
	}
	return nil
}

func (p *plugin) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_dns
			(slug, outpost_slug, duration_ms, pass, host, error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, "(error)", errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert dns error: %w", err)
	}
	return nil
}

func (p *plugin) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
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
			c           DNSCheck
			durationMS  sql.NullInt64
			responseMS  sql.NullFloat64
			host        sql.NullString
			ips         sql.NullString
			errMsg      sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &host, &ips, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dns check: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Host = host.String
		c.IPs = ips.String
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

func (p *plugin) ExtractPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]DNSCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = registry.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (p *plugin) ExtractDurationPoints(history interface{}) []registry.CheckPoint {
	return nil
}

func (p *plugin) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
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
	rec := make([]DNSCheck, n)
	for i := range n {
		rec[i] = h[len(h)-2-i]
	}
	return latest, rec, n
}

func (p *plugin) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)

		timeout := defaultTimeout
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			timeout = readIntOpt(l, 2, "timeout", defaultTimeout)
		}

		r := &DNSResult{
			Pass: protocol.FAIL,
			Host: host,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		start := time.Now()
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushDNSResult(l, r)
			return 1
		}

		r.IPs = ips
		pushDNSResult(l, r)
		return 1
	})
	l.SetGlobal("dns_lookup")
}

func (p *plugin) DispatchWireResult(res registry.ResourceMeta, cr protocol.CheckResult, elapsed time.Duration) protocol.WireResult {
	r := cr.(*DNSResult)
	return protocol.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"dns", r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error,
		r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

func (p *plugin) TemplateNames() (string, string) {
	return "check_dns_row", "check_dns_body"
}

func (p *plugin) TemplateFiles() []string {
	return []string{"templates/checks/dns.html"}
}

func init() {
	registry.Register(&plugin{})
}

// --- Lua helpers (copied from cmd/scoutpost/lmods) ---------------------------

func pushDNSResult(l *lua.State, r *DNSResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*DNSResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "Host":
			l.Push(r.Host)
		case "IPs":
			pushStrSlice(l, r.IPs)
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "Error":
			pushStr(l, r.Error)
		default:
			l.Push(nil)
		}
		return 1
	})
	l.SetTableRaw(-3)

	l.Push("__newindex")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*DNSResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "Host":
			r.Host = l.ToString(3)
		case "ResponseTimeMS":
			r.ResponseTimeMS = l.ToFloat(3)
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}

func pushStr(l *lua.State, s string) {
	if s == "" {
		l.Push(nil)
	} else {
		l.Push(s)
	}
}

func pushStrSlice(l *lua.State, strs []string) {
	l.NewTable(len(strs), 0)
	for i, s := range strs {
		l.Push(int64(i + 1))
		l.Push(s)
		l.SetTableRaw(-3)
	}
}

func readIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return def
	}
	switch n := l.GetRaw(-1).(type) {
	case int64:
		l.Pop(1)
		return int(n)
	case float64:
		l.Pop(1)
		return int(n)
	}
	l.Pop(1)
	return def
}
