// Package tcp implements registry.CheckPlugin for TCP connect checks.
package tcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// TCPResult is the result of a TCP connect check. It implements protocol.CheckResult.
type TCPResult struct {
	Pass           int
	FailReason     string
	Host           string
	Port           int
	ResponseTimeMS float64
	RemoteIP       string
	Error          string
}

func (r *TCPResult) CheckType() string        { return "tcp" }
func (r *TCPResult) CheckPass() int            { return r.Pass }
func (r *TCPResult) CheckFailReason() string   { return r.FailReason }
func (r *TCPResult) CheckResponseMS() float64  { return r.ResponseTimeMS }

// TCPCheck is a database row from checks_tcp.
type TCPCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	Host           string
	Port           int
	RemoteIP       string
	Error          string
}

type impl struct{}

func (impl) TypeName() string { return "tcp" }

func (impl) TableName() string { return "checks_tcp" }

func (impl) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_tcp (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			outpost_slug    TEXT NOT NULL DEFAULT '',
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			host            TEXT NOT NULL,
			port            INTEGER NOT NULL,
			remote_ip       TEXT,
			error           TEXT
		)`,
		`ALTER TABLE checks_tcp ADD COLUMN outpost_slug TEXT NOT NULL DEFAULT ''`,
	}
}

func (impl) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_tcp_slug_time ON checks_tcp(slug, timestamp)`,
	}
}

func (impl) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r TCPResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal tcp data: %w", err)
	}
	_, err := db.Exec(
		`INSERT INTO checks_tcp
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, port, remote_ip, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.Host, r.Port, r.RemoteIP, r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert tcp check: %w", err)
	}
	return nil
}

func (impl) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_tcp
			(slug, outpost_slug, duration_ms, pass, response_time_ms, host, port, remote_ip, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, 0.0, "(error)", 0, "", errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert tcp check error: %w", err)
	}
	return nil
}

func (impl) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			host, port, remote_ip, error
		FROM checks_tcp WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query tcp checks since: %w", err)
	}
	defer rows.Close()

	var checks []TCPCheck
	for rows.Next() {
		var (
			c          TCPCheck
			durationMS sql.NullInt64
			responseMS sql.NullFloat64
			host       sql.NullString
			port       sql.NullInt64
			remoteIP   sql.NullString
			errMsg     sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &host, &port, &remoteIP, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan tcp check since: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Host = host.String
		c.Port = int(port.Int64)
		c.RemoteIP = remoteIP.String
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows tcp check since: %w", err)
	}
	return checks, nil
}

func (impl) ExtractPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]TCPCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = registry.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (impl) ExtractDurationPoints(history interface{}) []registry.CheckPoint {
	return nil
}

func (impl) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]TCPCheck)
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
	reversed := make([]TCPCheck, n)
	for i := range n {
		reversed[i] = h[len(h)-2-i]
	}
	return latest, reversed, n
}

func (impl) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)
		port := int(l.ToInt(2))

		timeout := defaultTimeout
		if !l.IsNil(3) && l.TypeOf(3) == lua.TypTable {
			timeout = readIntOpt(l, 3, "timeout", defaultTimeout)
		}

		r := &TCPResult{
			Pass: protocol.FAIL,
			Host: host,
			Port: port,
		}

		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushTCPResult(l, r)
			return 1
		}
		defer conn.Close()

		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			r.RemoteIP = tcpAddr.IP.String()
		}

		pushTCPResult(l, r)
		return 1
	})
	l.SetGlobal("tcp_connect")
}

func pushTCPResult(l *lua.State, r *TCPResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*TCPResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "Host":
			l.Push(r.Host)
		case "Port":
			l.Push(int64(r.Port))
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "RemoteIP":
			pushStr(l, r.RemoteIP)
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
		r := l.ToUser(1).(*TCPResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "Host":
			r.Host = l.ToString(3)
		case "Port":
			r.Port = int(l.ToInt(3))
		case "RemoteIP":
			r.RemoteIP = l.ToString(3)
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}

// Lua utility helpers — mirrors of lmods unexported helpers.
func readIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
	if t == lua.TypNil {
		l.Pop(1)
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

func pushStr(l *lua.State, s string) {
	if s == "" {
		l.Push(nil)
	} else {
		l.Push(s)
	}
}

func (impl) DispatchWireResult(res registry.ResourceMeta, cr protocol.CheckResult, elapsed time.Duration) protocol.WireResult {
	r := cr.(*TCPResult)
	return protocol.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"tcp",
		r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error,
		r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

func (impl) TemplateNames() (string, string) {
	return "check_tcp_row", "check_tcp_body"
}



func init() {
	registry.Register(impl{})
}
