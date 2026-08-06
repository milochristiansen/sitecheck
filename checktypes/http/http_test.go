package http

import (
	"crypto/tls"
	"encoding/json"
	"testing"
	"time"

	"sitecheck/core"
)

func TestHTTPResultInterface(t *testing.T) {
	r := &HTTPResult{
		Pass:           core.PASS,
		FailReason:     "",
		ResponseTimeMS: 42.5,
		URL:            "https://example.com",
		StatusCode:     200,
	}

	if r.CheckType() != "http" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "http")
	}
	if r.CheckPass() != core.PASS {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), core.PASS)
	}
	if r.CheckFailReason() != "" {
		t.Errorf("CheckFailReason = %q, want empty", r.CheckFailReason())
	}
	if r.CheckResponseMS() != 42.5 {
		t.Errorf("CheckResponseMS = %f, want %f", r.CheckResponseMS(), 42.5)
	}
}

func TestHTTPCheckResponseMSZero(t *testing.T) {
	// Verify CheckResponseMS returns zero when ResponseTimeMS is unset.
	r := &HTTPResult{Pass: core.FAIL}
	if r.CheckResponseMS() != 0.0 {
		t.Errorf("CheckResponseMS = %f, want 0.0", r.CheckResponseMS())
	}
}

func TestHTTPPluginRegistration(t *testing.T) {
	p, ok := core.ByName("http")
	if !ok {
		t.Fatal("HTTP plugin not registered — did init() run?")
	}
	if p.TypeName() != "http" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "http")
	}
	if p.TableName() != "checks_http" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_http")
	}
}

func TestHTTPPluginCreateTableDDL(t *testing.T) {
	p, _ := core.ByName("http")
	ddl := p.CreateTableDDL()
	if len(ddl) == 0 {
		t.Fatalf("CreateTableDDL returned empty slice")
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestHTTPPluginCreateIndexDDL(t *testing.T) {
	p, _ := core.ByName("http")
	ddl := p.CreateIndexDDL()
	if len(ddl) == 0 {
		t.Fatalf("CreateIndexDDL returned empty slice")
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

func TestHTTPPluginTemplateNames(t *testing.T) {
	p, _ := core.ByName("http")
	row, body := p.TemplateNames()
	if row != "check_http_row" {
		t.Errorf("row template = %q, want %q", row, "check_http_row")
	}
	if body != "check_http_body" {
		t.Errorf("body template = %q, want %q", body, "check_http_body")
	}
}

func TestHTTPPluginDispatchWireResult(t *testing.T) {
	p, _ := core.ByName("http")
	meta := core.ResourceMeta{
		Slug:           "test-http",
		Name:           "Test HTTP",
		Desc:           "An HTTP check test",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &HTTPResult{
		Pass:           core.DEGRADED,
		FailReason:     "slow response",
		URL:            "https://example.com",
		StatusCode:     200,
		ResponseTimeMS: 850.3,
		BodySize:       1234,
		TLSVersion:     "TLS 1.3",
		RemoteIP:       "93.184.216.34",
		RedirectCount:  0,
		Error:          "",
	}
	elapsed := 950 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-http" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-http")
	}
	if wr.CheckType != "http" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "http")
	}
	if wr.Pass != core.DEGRADED {
		t.Errorf("Pass = %d, want %d", wr.Pass, core.DEGRADED)
	}
	if wr.FailReason != "slow response" {
		t.Errorf("FailReason = %q, want %q", wr.FailReason, "slow response")
	}
	if wr.ResponseMS != 850.3 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 850.3)
	}

	// Verify Data contains the HTTP-specific fields.
	var result HTTPResult
	if err := json.Unmarshal(wr.Data, &result); err != nil {
		t.Fatalf("Data unmarshal: %v", err)
	}
	if result.URL != "https://example.com" {
		t.Errorf("Data.URL = %q, want %q", result.URL, "https://example.com")
	}
	if result.StatusCode != 200 {
		t.Errorf("Data.StatusCode = %d, want %d", result.StatusCode, 200)
	}
	if result.TLSVersion != "TLS 1.3" {
		t.Errorf("Data.TLSVersion = %q, want %q", result.TLSVersion, "TLS 1.3")
	}
}

func TestHTTPPluginExtractPoints(t *testing.T) {
	p, _ := core.ByName("http")
	history := []HTTPCheck{
		{Pass: core.PASS, ResponseTimeMS: 10.0, Timestamp: "2024-01-01T00:00:00Z"},
		{Pass: core.FAIL, ResponseTimeMS: 0.0, Timestamp: "2024-01-01T00:01:00Z"},
	}
	pts := p.ExtractPoints(history)
	if len(pts) != 2 {
		t.Fatalf("ExtractPoints = %d points, want 2", len(pts))
	}
	if pts[0].Pass != core.PASS {
		t.Errorf("pts[0].Pass = %d, want %d", pts[0].Pass, core.PASS)
	}
	if pts[0].Resp != 10.0 {
		t.Errorf("pts[0].Resp = %f, want %f", pts[0].Resp, 10.0)
	}
	if pts[1].Pass != core.FAIL {
		t.Errorf("pts[1].Pass = %d, want %d", pts[1].Pass, core.FAIL)
	}
	if pts[1].Resp != 0.0 {
		t.Errorf("pts[1].Resp = %f, want 0.0", pts[1].Resp)
	}
	if pts[1].TS != "2024-01-01T00:01:00Z" {
		t.Errorf("pts[1].TS = %q, want %q", pts[1].TS, "2024-01-01T00:01:00Z")
	}
}

func TestHTTPPluginExtractPointsNil(t *testing.T) {
	p, _ := core.ByName("http")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

func TestHTTPPluginExtractPointsWrongType(t *testing.T) {
	p, _ := core.ByName("http")
	// Passing wrong type should return nil.
	pts := p.ExtractPoints("not a slice")
	if pts != nil {
		t.Errorf("ExtractPoints(wrong type) = %v, want nil", pts)
	}
}

func TestHTTPPluginExtractDurationPoints(t *testing.T) {
	p, _ := core.ByName("http")
	pts := p.ExtractDurationPoints([]HTTPCheck{})
	if pts != nil {
		t.Errorf("ExtractDurationPoints should return nil for HTTP")
	}
}

func TestHTTPPluginLatestRecent(t *testing.T) {
	p, _ := core.ByName("http")
	history := []HTTPCheck{
		{ID: 1, Pass: core.FAIL, URL: "https://a.example.com"},
		{ID: 2, Pass: core.PASS, URL: "https://b.example.com"},
		{ID: 3, Pass: core.DEGRADED, URL: "https://c.example.com"},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*HTTPCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3, got %v", l.ID)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]HTTPCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = want 2, got %d", len(rec))
	} else {
		// The recent slice is reversed: newest-first (excluding latest).
		if rec[0].ID != 2 {
			t.Errorf("rec[0].ID = %d, want 2 (newest recent)", rec[0].ID)
		}
		if rec[1].ID != 1 {
			t.Errorf("rec[1].ID = %d, want 1 (oldest recent)", rec[1].ID)
		}
	}
}

func TestHTTPPluginLatestRecentSingle(t *testing.T) {
	p, _ := core.ByName("http")
	history := []HTTPCheck{
		{ID: 1, Pass: core.PASS, URL: "https://example.com"},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*HTTPCheck); !ok || l.ID != 1 {
		t.Errorf("latest.ID = want 1, got %v", l.ID)
	}
	if recent != nil {
		t.Errorf("recent should be nil for single entry, got %v", recent)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for single entry", count)
	}
}

func TestHTTPPluginLatestRecentEmpty(t *testing.T) {
	p, _ := core.ByName("http")
	latest, recent, count := p.LatestRecent([]HTTPCheck{})
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history")
	}
}

func TestHTTPPluginLatestRecentUncapped(t *testing.T) {
	p, _ := core.ByName("http")
	history := make([]HTTPCheck, 10)
	for i := range history {
		history[i] = HTTPCheck{ID: int64(i + 1), Pass: core.PASS}
	}
	// All nine preceding checks are returned, newest-first.
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*HTTPCheck); !ok || l.ID != 10 {
		t.Errorf("latest.ID = want 10, got %v", l.ID)
	}
	if count != 9 {
		t.Errorf("count = %d, want 9 (all preceding checks)", count)
	}
	rec, ok := recent.([]HTTPCheck)
	if !ok || len(rec) != 9 {
		t.Fatalf("recent length = want 9, got %d", len(rec))
	}
	if rec[0].ID != 9 || rec[8].ID != 1 {
		t.Errorf("recent order = want [9..1], got first=%d last=%d", rec[0].ID, rec[8].ID)
	}
}

func TestTLSVersionName(t *testing.T) {
	tests := []struct {
		v    uint16
		want string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0304, "TLS 1.3"}, // tls.VersionTLS13 alias
		{0x0000, "TLS 0x0000"},
		{0xffff, "TLS 0xffff"},
	}
	for _, tt := range tests {
		got := tlsVersionName(tt.v)
		if got != tt.want {
			t.Errorf("tlsVersionName(0x%04x) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestDispatchWireResultCheckResponseMS(t *testing.T) {
	// Verify the ResponseMS in the WireResult matches CheckResponseMS from the result.
	p, _ := core.ByName("http")
	meta := core.ResourceMeta{Slug: "x", Name: "x", Desc: ""}
	cr := &HTTPResult{Pass: core.PASS, ResponseTimeMS: 123.456}
	wr := p.DispatchWireResult(meta, cr, 100*time.Millisecond)
	if wr.ResponseMS != 123.456 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 123.456)
	}
}

func TestHTTPResultFailReason(t *testing.T) {
	r := &HTTPResult{
		Pass:       core.FAIL,
		FailReason: "connection refused",
		Error:      "dial tcp 127.0.0.1:80: connect: connection refused",
	}
	if r.CheckPass() != core.FAIL {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), core.FAIL)
	}
	if r.CheckFailReason() != "connection refused" {
		t.Errorf("CheckFailReason = %q, want %q", r.CheckFailReason(), "connection refused")
	}
}

func TestInsertInvalidJSON(t *testing.T) {
	p, _ := core.ByName("http")
	err := p.Insert(nil, "slug", "outpost", 100, []byte(`not json`))
	if err == nil {
		t.Error("Insert with invalid JSON should return an error")
	}
}
