package dns

import (
	"encoding/json"
	"testing"
	"time"

	"sitecheck/core"
)

func TestDNSResultInterface(t *testing.T) {
	r := &DNSResult{
		Pass:       core.PASS,
		FailReason: "",
		Host:       "example.com",
		IPs:        "93.184.216.34",
	}

	if r.CheckType() != "dns" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "dns")
	}
	if r.CheckPass() != core.PASS {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), core.PASS)
	}
	if r.CheckFailReason() != "" {
		t.Errorf("CheckFailReason = %q, want empty", r.CheckFailReason())
	}
}

func TestDNSPluginRegistration(t *testing.T) {
	p, ok := core.ByName("dns")
	if !ok {
		t.Fatal("DNS plugin not registered — did init() run?")
	}
	if p.TypeName() != "dns" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "dns")
	}
	if p.TableName() != "checks_dns" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_dns")
	}
}

func TestDNSPluginCreateTableDDL(t *testing.T) {
	p, _ := core.ByName("dns")
	ddl := p.CreateTableDDL()
	if len(ddl) != 2 {
		t.Fatalf("CreateTableDDL returned %d statements, want 2", len(ddl))
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestDNSPluginCreateIndexDDL(t *testing.T) {
	p, _ := core.ByName("dns")
	ddl := p.CreateIndexDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateIndexDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

func TestDNSPluginDispatchWireResult(t *testing.T) {
	p, _ := core.ByName("dns")
	meta := core.ResourceMeta{
		Slug:           "test-dns",
		Name:           "Test DNS",
		Desc:           "A test",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &DNSResult{
		Pass:           core.DEGRADED,
		FailReason:     "slow resolution",
		Host:           "example.com",
		IPs:            "1.2.3.4, 5.6.7.8",
		ResponseTimeMS: 150.5,
		Error:          "",
	}
	elapsed := 200 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-dns" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-dns")
	}
	if wr.CheckType != "dns" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "dns")
	}
	if wr.Pass != core.DEGRADED {
		t.Errorf("Pass = %d, want %d", wr.Pass, core.DEGRADED)
	}
	if wr.FailReason != "slow resolution" {
		t.Errorf("FailReason = %q, want %q", wr.FailReason, "slow resolution")
	}
	if wr.ResponseMS != 150.5 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 150.5)
	}

	// Verify Data contains the IPs field.
	var result DNSResult
	if err := json.Unmarshal(wr.Data, &result); err != nil {
		t.Fatalf("Data unmarshal: %v", err)
	}
	if result.IPs != "1.2.3.4, 5.6.7.8" {
		t.Errorf("Data.IPs = %q, want %q", result.IPs, "1.2.3.4, 5.6.7.8")
	}
}

func TestDNSPluginExtractPoints(t *testing.T) {
	p, _ := core.ByName("dns")
	history := []DNSCheck{
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
	if pts[1].Resp != 0.0 {
		t.Errorf("pts[1].Resp = %f, want 0.0", pts[1].Resp)
	}
}

func TestDNSPluginExtractPointsNil(t *testing.T) {
	p, _ := core.ByName("dns")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

func TestDNSPluginExtractDurationPoints(t *testing.T) {
	p, _ := core.ByName("dns")
	pts := p.ExtractDurationPoints([]DNSCheck{})
	if pts != nil {
		t.Errorf("ExtractDurationPoints should return nil for DNS")
	}
}

func TestDNSPluginLatestRecent(t *testing.T) {
	p, _ := core.ByName("dns")
	history := []DNSCheck{
		{ID: 1, Pass: core.FAIL},
		{ID: 2, Pass: core.PASS},
		{ID: 3, Pass: core.DEGRADED},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*DNSCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]DNSCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = want 2")
	}
}

func TestDNSPluginLatestRecentEmpty(t *testing.T) {
	p, _ := core.ByName("dns")
	latest, recent, count := p.LatestRecent([]DNSCheck{})
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history")
	}
}

func TestDNSPluginTemplateNames(t *testing.T) {
	p, _ := core.ByName("dns")
	row, body := p.TemplateNames()
	if row != "check_dns_row" {
		t.Errorf("row template = %q, want %q", row, "check_dns_row")
	}
	if body != "check_dns_body" {
		t.Errorf("body template = %q, want %q", body, "check_dns_body")
	}
}
