// Package http implements the registry.CheckPlugin interface for HTTP checks.
package http

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// HTTPResult is the result of a single HTTP check. It implements protocol.CheckResult.
type HTTPResult struct {
	Pass           int
	FailReason     string
	URL            string
	StatusCode     int
	Body           string
	BodySize       int64
	ResponseTimeMS float64
	TLSVersion     string
	RemoteIP       string
	RedirectCount  int
	Error          string
}

func (r *HTTPResult) CheckType() string        { return "http" }
func (r *HTTPResult) CheckPass() int           { return r.Pass }
func (r *HTTPResult) CheckFailReason() string  { return r.FailReason }
func (r *HTTPResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// HTTPCheck is a single row from checks_http.
type HTTPCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	StatusCode     int
	URL            string
	Body           *string
	BodySize       int64
	TLSVersion     string
	RemoteIP       string
	RedirectCount  int
	Error          string
}

// HTTPPlugin implements registry.CheckPlugin for HTTP checks.
type HTTPPlugin struct{}

// TypeName returns "http".
func (p *HTTPPlugin) TypeName() string { return "http" }

// TableName returns the database table name.
func (p *HTTPPlugin) TableName() string { return "checks_http" }

// CreateTableDDL returns the CREATE TABLE statement for checks_http.
func (p *HTTPPlugin) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_http (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			outpost_slug    TEXT NOT NULL DEFAULT '',
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			status_code     INTEGER,
			url             TEXT NOT NULL,
			body_size       INTEGER,
			body            TEXT,
			tls_version     TEXT,
			remote_ip       TEXT,
			redirect_count  INTEGER,
			error           TEXT
		)`,
	}
}

// CreateIndexDDL returns the index statements for checks_http.
func (p *HTTPPlugin) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_http_slug_time ON checks_http(slug, timestamp)`,
	}
}

// Insert unmarshals data into HTTPResult and inserts a row into checks_http.
func (p *HTTPPlugin) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r HTTPResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal http data: %w", err)
	}
	return p.insert(db, slug, outpostSlug, elapsedMS, &r)
}

func (p *HTTPPlugin) insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, r *HTTPResult) error {
	var body *string
	if r.Body != "" {
		body = &r.Body
	}
	_, err := db.Exec(
		`INSERT INTO checks_http
			(slug, outpost_slug, duration_ms, pass, response_time_ms, status_code, url, body, body_size, tls_version, remote_ip, redirect_count, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS, r.StatusCode, r.URL,
		body, r.BodySize, r.TLSVersion, r.RemoteIP, r.RedirectCount, r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert http check: %w", err)
	}
	return nil
}

// InsertError inserts a minimal error row into checks_http.
func (p *HTTPPlugin) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_http (slug, outpost_slug, duration_ms, pass, error, url)
		VALUES (?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, errMsg, "(error)",
	)
	if err != nil {
		return fmt.Errorf("insert http error check: %w", err)
	}
	return nil
}

// QuerySince returns HTTPCheck rows since the given time, ordered oldest first.
func (p *HTTPPlugin) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			status_code, url, body, body_size, tls_version, remote_ip, redirect_count, error
		FROM checks_http WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query http checks since: %w", err)
	}
	defer rows.Close()

	var checks []HTTPCheck
	for rows.Next() {
		var (
			c           HTTPCheck
			durationMS  sql.NullInt64
			responseMS  sql.NullFloat64
			statusCode  sql.NullInt64
			url         sql.NullString
			body        sql.NullString
			bodySize    sql.NullInt64
			tlsVersion  sql.NullString
			remoteIP    sql.NullString
			redirectCnt sql.NullInt64
			errMsg      sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &statusCode, &url, &body, &bodySize,
			&tlsVersion, &remoteIP, &redirectCnt, &errMsg,
		)
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.StatusCode = int(statusCode.Int64)
		c.URL = url.String
		if body.Valid {
			c.Body = &body.String
		}
		c.BodySize = bodySize.Int64
		c.TLSVersion = tlsVersion.String
		c.RemoteIP = remoteIP.String
		c.RedirectCount = int(redirectCnt.Int64)
		c.Error = errMsg.String
		if err != nil {
			return nil, fmt.Errorf("scan http check since: %w", err)
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// ExtractPoints converts a []HTTPCheck to []registry.CheckPoint.
func (p *HTTPPlugin) ExtractPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]HTTPCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = registry.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

// ExtractDurationPoints returns nil — HTTP checks don't have duration charts.
func (p *HTTPPlugin) ExtractDurationPoints(_ interface{}) []registry.CheckPoint {
	return nil
}

// LatestRecent returns the newest check, and the rest reversed (newest-first, capped at maxRecent).
func (p *HTTPPlugin) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]HTTPCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	if len(h) <= 1 {
		return latest, nil, 0
	}
	n := len(h) - 1
	if n > maxRecent {
		n = maxRecent
	}
	reversed := make([]HTTPCheck, n)
	for i := range n {
		reversed[i] = h[len(h)-2-i]
	}
	return latest, reversed, n
}

// RegisterLua registers the "http_fetch" global function in the Lua state.
func (p *HTTPPlugin) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		url := l.ToString(1)

		method := "GET"
		timeout := defaultTimeout
		followRedirects := true
		maxRedirects := 10
		insecureSkipVerify := false
		var headers map[string]string
		var body string

		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			method = readStringOpt(l, 2, "method", "GET")
			timeout = readIntOpt(l, 2, "timeout", defaultTimeout)
			followRedirects = readBoolOpt(l, 2, "follow_redirects", true)
			maxRedirects = readIntOpt(l, 2, "max_redirects", 10)
			insecureSkipVerify = readBoolOpt(l, 2, "insecure_skip_verify", false)
			headers = readStringMapOpt(l, 2, "headers")
			body = readStringOpt(l, 2, "body", "")
		}

		r := &HTTPResult{
			Pass: protocol.FAIL,
			URL:  url,
		}

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		}

		redirectCount := 0

		client := &http.Client{
			Timeout:   time.Duration(timeout) * time.Second,
			Transport: transport,
		}

		if !followRedirects {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		} else {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				redirectCount = len(via)
				if len(via) >= maxRedirects {
					return fmt.Errorf("too many redirects (%d)", maxRedirects)
				}
				return nil
			}
		}

		var reqBody io.Reader
		if body != "" {
			reqBody = strings.NewReader(body)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			r.Error = err.Error()
			pushHTTPResult(l, r)
			return 1
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushHTTPResult(l, r)
			return 1
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

		r.StatusCode = resp.StatusCode
		r.Body = string(bodyBytes)
		r.BodySize = int64(len(bodyBytes))
		r.RedirectCount = redirectCount

		if resp.TLS != nil {
			r.TLSVersion = tlsVersionName(resp.TLS.Version)
		}

		if resp.Request != nil && resp.Request.URL != nil {
			r.URL = resp.Request.URL.String()
		}

		if resp.Request != nil && resp.Request.URL != nil {
			host := resp.Request.URL.Hostname()
			if ips, err := net.LookupHost(host); err == nil && len(ips) > 0 {
				r.RemoteIP = ips[0]
			}
		}

		pushHTTPResult(l, r)
		return 1
	})
	l.SetGlobal("http_fetch")
}

// pushHTTPResult pushes *HTTPResult as userdata with an explicit metatable.
func pushHTTPResult(l *lua.State, r *HTTPResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*HTTPResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "URL":
			l.Push(r.URL)
		case "StatusCode":
			l.Push(int64(r.StatusCode))
		case "BodySize":
			l.Push(r.BodySize)
		case "Body":
			l.Push(r.Body)
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "TLSVersion":
			pushStr(l, r.TLSVersion)
		case "RemoteIP":
			pushStr(l, r.RemoteIP)
		case "RedirectCount":
			l.Push(int64(r.RedirectCount))
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
		r := l.ToUser(1).(*HTTPResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "URL":
			r.URL = l.ToString(3)
		case "Body":
			r.Body = l.ToString(3)
		case "TLSVersion":
			r.TLSVersion = l.ToString(3)
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

// pushStr pushes s as a Lua string, or nil if s is empty.
func pushStr(l *lua.State, s string) {
	if s == "" {
		l.Push(nil)
	} else {
		l.Push(s)
	}
}

// tlsVersionName returns a human-readable name for a TLS version constant.
func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("TLS 0x%04x", v)
	}
}

// DispatchWireResult builds a protocol.WireResult from the check result.
func (p *HTTPPlugin) DispatchWireResult(res registry.ResourceMeta, cr protocol.CheckResult, elapsed time.Duration) protocol.WireResult {
	r := cr.(*HTTPResult)
	return protocol.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"http", r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error, r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

// TemplateNames returns the template names for HTTP check rows and body.
func (p *HTTPPlugin) TemplateNames() (string, string) {
	return "check_http_row", "check_http_body"
}



func init() {
	registry.Register(&HTTPPlugin{})
}

// --- Lua option helpers (ported from lmods/opts.go) ---

func readOptional(l *lua.State, tableIdx int, key string) interface{} {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return nil
	}
	v := l.GetRaw(-1)
	l.Pop(1)
	return v
}

func readStringOpt(l *lua.State, tableIdx int, key string, def string) string {
	v := readOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func readIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	v := readOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

func readBoolOpt(l *lua.State, tableIdx int, key string, def bool) bool {
	v := readOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func readStringMapOpt(l *lua.State, tableIdx int, key string) map[string]string {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil || t != lua.TypTable {
		l.Pop(1)
		return nil
	}

	result := make(map[string]string)
	subIdx := l.AbsIndex(-1)

	l.ForEachRaw(subIdx, func() bool {
		k := l.GetRaw(-2)
		if ks, ok := k.(string); ok {
			v := l.GetRaw(-1)
			if vs, ok := v.(string); ok {
				result[ks] = vs
			}
		}
		return true
	})

	l.Pop(1)
	if len(result) == 0 {
		return nil
	}
	return result
}
