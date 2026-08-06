package tcp

import (
	"encoding/json"
	"testing"
	"time"

	"sitecheck/core"
)

func TestTCPResultInterface(t *testing.T) {
	r := &TCPResult{
		Pass:           core.PASS,
		FailReason:     "",
		Host:           "example.com",
		Port:           443,
		ResponseTimeMS: 10.5,
		RemoteIP:       "93.184.216.34",
	}

	if r.CheckType() != "tcp" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "tcp")
	}
	if r.CheckPass() != core.PASS {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), core.PASS)
	}
	if r.CheckFailReason() != "" {
		t.Errorf("CheckFailReason = %q, want empty", r.CheckFailReason())
	}
	if r.CheckResponseMS() != 10.5 {
		t.Errorf("CheckResponseMS = %f, want %f", r.CheckResponseMS(), 10.5)
	}
}

func TestTCPPluginRegistration(t *testing.T) {
	p, ok := core.ByName("tcp")
	if !ok {
		t.Fatal("TCP plugin not registered — did init() run?")
	}
	if p.TypeName() != "tcp" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "tcp")
	}
	if p.TableName() != "checks_tcp" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_tcp")
	}
}

func TestTCPPluginCreateTableDDL(t *testing.T) {
	p, _ := core.ByName("tcp")
	ddl := p.CreateTableDDL()
	if len(ddl) != 2 {
		t.Fatalf("CreateTableDDL returned %d statements, want 2", len(ddl))
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestTCPPluginCreateIndexDDL(t *testing.T) {
	p, _ := core.ByName("tcp")
	ddl := p.CreateIndexDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateIndexDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

func TestTCPPluginDispatchWireResult(t *testing.T) {
	p, _ := core.ByName("tcp")
	meta := core.ResourceMeta{
		Slug:           "test-tcp",
		Name:           "Test TCP",
		Desc:           "A test",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &TCPResult{
		Pass:           core.DEGRADED,
		FailReason:     "connection slow",
		Host:           "example.com",
		Port:           443,
		ResponseTimeMS: 250.75,
		RemoteIP:       "93.184.216.34",
		Error:          "",
	}
	elapsed := 300 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-tcp" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-tcp")
	}
	if wr.CheckType != "tcp" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "tcp")
	}
	if wr.Pass != core.DEGRADED {
		t.Errorf("Pass = %d, want %d", wr.Pass, core.DEGRADED)
	}
	if wr.FailReason != "connection slow" {
		t.Errorf("FailReason = %q, want %q", wr.FailReason, "connection slow")
	}
	if wr.ResponseMS != 250.75 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 250.75)
	}

	// Verify Data contains the RemoteIP and Port fields.
	var result TCPResult
	if err := json.Unmarshal(wr.Data, &result); err != nil {
		t.Fatalf("Data unmarshal: %v", err)
	}
	if result.RemoteIP != "93.184.216.34" {
		t.Errorf("Data.RemoteIP = %q, want %q", result.RemoteIP, "93.184.216.34")
	}
	if result.Port != 443 {
		t.Errorf("Data.Port = %d, want %d", result.Port, 443)
	}
}

func TestTCPPluginExtractPoints(t *testing.T) {
	p, _ := core.ByName("tcp")
	history := []TCPCheck{
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

func TestTCPPluginExtractPointsNil(t *testing.T) {
	p, _ := core.ByName("tcp")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

func TestTCPPluginExtractDurationPoints(t *testing.T) {
	p, _ := core.ByName("tcp")
	pts := p.ExtractDurationPoints([]TCPCheck{})
	if pts != nil {
		t.Errorf("ExtractDurationPoints should return nil for TCP")
	}
}

func TestTCPPluginLatestRecent(t *testing.T) {
	p, _ := core.ByName("tcp")
	history := []TCPCheck{
		{ID: 1, Pass: core.FAIL},
		{ID: 2, Pass: core.PASS},
		{ID: 3, Pass: core.DEGRADED},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*TCPCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3, got %d", l.ID)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]TCPCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = %d, want 2", len(rec))
	}
}

func TestTCPPluginLatestRecentEmpty(t *testing.T) {
	p, _ := core.ByName("tcp")
	latest, recent, count := p.LatestRecent([]TCPCheck{})
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history")
	}
}

func TestTCPPluginLatestRecentSingle(t *testing.T) {
	p, _ := core.ByName("tcp")
	history := []TCPCheck{
		{ID: 42, Pass: core.PASS, Host: "example.com", Port: 443},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*TCPCheck); !ok || l.ID != 42 {
		t.Errorf("latest.ID = want 42, got %d", l.ID)
	}
	// Single entry: recent is nil, count is 0.
	if recent != nil {
		t.Errorf("recent should be nil for single entry")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for single entry", count)
	}
}

func TestTCPPluginTemplateNames(t *testing.T) {
	p, _ := core.ByName("tcp")
	row, body := p.TemplateNames()
	if row != "check_tcp_row" {
		t.Errorf("row template = %q, want %q", row, "check_tcp_row")
	}
	if body != "check_tcp_body" {
		t.Errorf("body template = %q, want %q", body, "check_tcp_body")
	}
}
