// Package http implements the core.CheckPlugin interface for HTTP checks.
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
	"sitecheck/core"
)

// HTTPResult is the result of a single HTTP check. It implements core.CheckResult.
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

// HTTPPlugin implements core.CheckPlugin for HTTP checks.
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

// ExtractPoints converts a []HTTPCheck to []core.CheckPoint.
func (p *HTTPPlugin) ExtractPoints(history interface{}) []core.CheckPoint {
	h, ok := history.([]HTTPCheck)
	if !ok {
		return nil
	}
	pts := make([]core.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = core.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

// ExtractDurationPoints returns nil — HTTP checks don't have duration charts.
func (p *HTTPPlugin) ExtractDurationPoints(_ interface{}) []core.CheckPoint {
	return nil
}

// LatestRecent returns the newest check, and all preceding checks reversed (newest-first).
func (p *HTTPPlugin) LatestRecent(history interface{}) (latest, recent interface{}, count int) {
	h, ok := history.([]HTTPCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	if len(h) <= 1 {
		return latest, nil, 0
	}
	n := len(h) - 1
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
			method = core.ReadStringOpt(l, 2, "method", "GET")
			timeout = core.ReadIntOpt(l, 2, "timeout", defaultTimeout)
			followRedirects = core.ReadBoolOpt(l, 2, "follow_redirects", true)
			maxRedirects = core.ReadIntOpt(l, 2, "max_redirects", 10)
			insecureSkipVerify = core.ReadBoolOpt(l, 2, "insecure_skip_verify", false)
			headers = core.ReadStringMapOpt(l, 2, "headers")
			body = core.ReadStringOpt(l, 2, "body", "")
		}

		r := &HTTPResult{
			Pass: core.FAIL,
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
		r.ResponseTimeMS = elapsed.Seconds() * 1000

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
	core.PushResultUserData(l, r, map[string]core.LuaField{
		"Pass":           {Get: func(l *lua.State) { l.Push(int64(r.Pass)) }, Set: func(l *lua.State) { r.Pass = int(l.ToInt(3)) }},
		"FailReason":     {Get: func(l *lua.State) { l.Push(r.FailReason) }, Set: func(l *lua.State) { r.FailReason = l.ToString(3) }},
		"URL":            {Get: func(l *lua.State) { l.Push(r.URL) }, Set: func(l *lua.State) { r.URL = l.ToString(3) }},
		"StatusCode":     {Get: func(l *lua.State) { l.Push(int64(r.StatusCode)) }},
		"BodySize":       {Get: func(l *lua.State) { l.Push(r.BodySize) }},
		"Body":           {Get: func(l *lua.State) { l.Push(r.Body) }, Set: func(l *lua.State) { r.Body = l.ToString(3) }},
		"ResponseTimeMS": {Get: func(l *lua.State) { l.Push(r.ResponseTimeMS) }},
		"TLSVersion":     {Get: func(l *lua.State) { l.Push(r.TLSVersion) }, Set: func(l *lua.State) { r.TLSVersion = l.ToString(3) }},
		"RemoteIP":       {Get: func(l *lua.State) { l.Push(r.RemoteIP) }, Set: func(l *lua.State) { r.RemoteIP = l.ToString(3) }},
		"RedirectCount":  {Get: func(l *lua.State) { l.Push(int64(r.RedirectCount)) }},
		"Error":          {Get: func(l *lua.State) { l.Push(r.Error) }, Set: func(l *lua.State) { r.Error = l.ToString(3) }},
	})
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

// DispatchWireResult builds a core.WireResult from the check result.
func (p *HTTPPlugin) DispatchWireResult(res core.ResourceMeta, cr core.CheckResult, elapsed time.Duration) core.WireResult {
	r := cr.(*HTTPResult)
	return core.NewWireResult(
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
	core.Register(&HTTPPlugin{})
}
