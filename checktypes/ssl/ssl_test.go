package ssl

import (
	"encoding/json"
	"testing"
	"time"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

func TestSSLResultInterface(t *testing.T) {
	r := &SSLResult{
		Pass:           protocol.PASS,
		FailReason:     "",
		Host:           "example.com",
		Port:           443,
		Issuer:         "CA Inc",
		Subject:        "example.com",
		NotBefore:      "2024-01-01",
		NotAfter:       "2025-01-01",
		DaysRemaining:  180,
		ResponseTimeMS: 50.0,
	}

	if r.CheckType() != "ssl" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "ssl")
	}
	if r.CheckPass() != protocol.PASS {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), protocol.PASS)
	}
	if r.CheckFailReason() != "" {
		t.Errorf("CheckFailReason = %q, want empty", r.CheckFailReason())
	}
	if r.CheckResponseMS() != 50.0 {
		t.Errorf("CheckResponseMS = %f, want %f", r.CheckResponseMS(), 50.0)
	}
}

func TestSSLPluginRegistration(t *testing.T) {
	p, ok := registry.ByName("ssl")
	if !ok {
		t.Fatal("SSL plugin not registered — did init() run?")
	}
	if p.TypeName() != "ssl" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "ssl")
	}
	if p.TableName() != "checks_ssl" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_ssl")
	}
}

func TestSSLPluginCreateTableDDL(t *testing.T) {
	p, _ := registry.ByName("ssl")
	ddl := p.CreateTableDDL()
	if len(ddl) != 2 {
		t.Fatalf("CreateTableDDL returned %d statements, want 2", len(ddl))
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestSSLPluginCreateIndexDDL(t *testing.T) {
	p, _ := registry.ByName("ssl")
	ddl := p.CreateIndexDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateIndexDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

func TestSSLPluginDispatchWireResult(t *testing.T) {
	p, _ := registry.ByName("ssl")
	meta := registry.ResourceMeta{
		Slug:           "test-ssl",
		Name:           "Test SSL",
		Desc:           "A test",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &SSLResult{
		Pass:           protocol.DEGRADED,
		FailReason:     "certificate expiring soon",
		Host:           "example.com",
		Port:           443,
		Issuer:         "CA Inc",
		Subject:        "example.com",
		NotBefore:      "2024-01-01",
		NotAfter:       "2025-01-01",
		DaysRemaining:  10,
		ResponseTimeMS: 150.5,
		Error:          "",
	}
	elapsed := 200 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-ssl" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-ssl")
	}
	if wr.CheckType != "ssl" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "ssl")
	}
	if wr.Pass != protocol.DEGRADED {
		t.Errorf("Pass = %d, want %d", wr.Pass, protocol.DEGRADED)
	}
	if wr.FailReason != "certificate expiring soon" {
		t.Errorf("FailReason = %q, want %q", wr.FailReason, "certificate expiring soon")
	}
	if wr.ResponseMS != 150.5 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 150.5)
	}

	// Verify Data contains the SSL-specific fields.
	var result SSLResult
	if err := json.Unmarshal(wr.Data, &result); err != nil {
		t.Fatalf("Data unmarshal: %v", err)
	}
	if result.Issuer != "CA Inc" {
		t.Errorf("Data.Issuer = %q, want %q", result.Issuer, "CA Inc")
	}
}

func TestSSLPluginExtractPoints(t *testing.T) {
	p, _ := registry.ByName("ssl")
	history := []SSLCheck{
		{Pass: protocol.PASS, ResponseTimeMS: 10.0, Timestamp: "2024-01-01T00:00:00Z"},
		{Pass: protocol.FAIL, ResponseTimeMS: 0.0, Timestamp: "2024-01-01T00:01:00Z"},
	}
	pts := p.ExtractPoints(history)
	if len(pts) != 2 {
		t.Fatalf("ExtractPoints = %d points, want 2", len(pts))
	}
	if pts[0].Pass != protocol.PASS {
		t.Errorf("pts[0].Pass = %d, want %d", pts[0].Pass, protocol.PASS)
	}
	if pts[1].Resp != 0.0 {
		t.Errorf("pts[1].Resp = %f, want 0.0", pts[1].Resp)
	}
}

func TestSSLPluginExtractPointsNil(t *testing.T) {
	p, _ := registry.ByName("ssl")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

func TestSSLPluginExtractDurationPoints(t *testing.T) {
	p, _ := registry.ByName("ssl")
	pts := p.ExtractDurationPoints([]SSLCheck{})
	if pts != nil {
		t.Errorf("ExtractDurationPoints should return nil for SSL")
	}
}

func TestSSLPluginLatestRecent(t *testing.T) {
	p, _ := registry.ByName("ssl")
	history := []SSLCheck{
		{ID: 1, Pass: protocol.FAIL},
		{ID: 2, Pass: protocol.PASS},
		{ID: 3, Pass: protocol.DEGRADED},
	}
	latest, recent, count := p.LatestRecent(history, 15)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*SSLCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]SSLCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = want 2")
	}
}

func TestSSLPluginLatestRecentEmpty(t *testing.T) {
	p, _ := registry.ByName("ssl")
	latest, recent, count := p.LatestRecent([]SSLCheck{}, 15)
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history")
	}
}

func TestSSLPluginTemplateNames(t *testing.T) {
	p, _ := registry.ByName("ssl")
	row, body := p.TemplateNames()
	if row != "check_ssl_row" {
		t.Errorf("row template = %q, want %q", row, "check_ssl_row")
	}
	if body != "check_ssl_body" {
		t.Errorf("body template = %q, want %q", body, "check_ssl_body")
	}
}
