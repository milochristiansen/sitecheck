package ping

import (
	"encoding/json"
	"testing"
	"time"

	"sitecheck/core"
)

func TestPingResultInterface(t *testing.T) {
	r := &PingResult{
		Pass:            core.PASS,
		FailReason:      "",
		Host:            "8.8.8.8",
		PacketsSent:     5,
		PacketsReceived: 5,
		PacketLossPct:   0.0,
		MinMS:           10.0,
		MaxMS:           30.0,
		ResponseTimeMS:  15.0,
	}

	if r.CheckType() != "ping" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "ping")
	}
	if r.CheckPass() != core.PASS {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), core.PASS)
	}
	if r.CheckFailReason() != "" {
		t.Errorf("CheckFailReason = %q, want empty", r.CheckFailReason())
	}
	if r.CheckResponseMS() != 15.0 {
		t.Errorf("CheckResponseMS = %f, want %f", r.CheckResponseMS(), 15.0)
	}
}

func TestPingPluginRegistration(t *testing.T) {
	p, ok := core.ByName("ping")
	if !ok {
		t.Fatal("Ping plugin not registered — did init() run?")
	}
	if p.TypeName() != "ping" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "ping")
	}
	if p.TableName() != "checks_ping" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_ping")
	}
}

func TestPingPluginCreateTableDDL(t *testing.T) {
	p, _ := core.ByName("ping")
	ddl := p.CreateTableDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateTableDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestPingPluginCreateIndexDDL(t *testing.T) {
	p, _ := core.ByName("ping")
	ddl := p.CreateIndexDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateIndexDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

func TestPingPluginDispatchWireResult(t *testing.T) {
	p, _ := core.ByName("ping")
	meta := core.ResourceMeta{
		Slug:           "test-ping",
		Name:           "Test Ping",
		Desc:           "A test ping check",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &PingResult{
		Pass:            core.DEGRADED,
		FailReason:      "high packet loss",
		Host:            "8.8.8.8",
		PacketsSent:     5,
		PacketsReceived: 3,
		PacketLossPct:   40.0,
		MinMS:           10.0,
		MaxMS:           30.0,
		ResponseTimeMS:  20.0,
		Error:           "",
	}
	elapsed := 200 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-ping" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-ping")
	}
	if wr.CheckType != "ping" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "ping")
	}
	if wr.Pass != core.DEGRADED {
		t.Errorf("Pass = %d, want %d", wr.Pass, core.DEGRADED)
	}
	if wr.FailReason != "high packet loss" {
		t.Errorf("FailReason = %q, want %q", wr.FailReason, "high packet loss")
	}
	if wr.ResponseMS != 20.0 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 20.0)
	}

	// Verify Data contains the host and packet fields.
	var result PingResult
	if err := json.Unmarshal(wr.Data, &result); err != nil {
		t.Fatalf("Data unmarshal: %v", err)
	}
	if result.Host != "8.8.8.8" {
		t.Errorf("Data.Host = %q, want %q", result.Host, "8.8.8.8")
	}
	if result.PacketsSent != 5 {
		t.Errorf("Data.PacketsSent = %d, want %d", result.PacketsSent, 5)
	}
	if result.PacketLossPct != 40.0 {
		t.Errorf("Data.PacketLossPct = %f, want %f", result.PacketLossPct, 40.0)
	}
}

func TestPingPluginExtractPoints(t *testing.T) {
	p, _ := core.ByName("ping")
	history := []PingCheck{
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
	if pts[1].TS != "2024-01-01T00:01:00Z" {
		t.Errorf("pts[1].TS = %q, want %q", pts[1].TS, "2024-01-01T00:01:00Z")
	}
}

func TestPingPluginExtractPointsNil(t *testing.T) {
	p, _ := core.ByName("ping")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

func TestPingPluginExtractDurationPoints(t *testing.T) {
	p, _ := core.ByName("ping")
	pts := p.ExtractDurationPoints([]PingCheck{})
	if pts != nil {
		t.Errorf("ExtractDurationPoints should return nil for ping")
	}
}

func TestPingPluginLatestRecent(t *testing.T) {
	p, _ := core.ByName("ping")
	history := []PingCheck{
		{ID: 1, Pass: core.FAIL, Timestamp: "2024-01-01T00:00:00Z"},
		{ID: 2, Pass: core.PASS, Timestamp: "2024-01-01T00:01:00Z"},
		{ID: 3, Pass: core.DEGRADED, Timestamp: "2024-01-01T00:02:00Z"},
	}
	latest, recent, count := p.LatestRecent(history, 15)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*PingCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3, got %d", l.ID)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]PingCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = %d, want 2", len(rec))
	}
	// Verify recent contains the two entries before latest, in reverse order.
	if rec, ok := recent.([]PingCheck); ok {
		if rec[0].ID != 2 {
			t.Errorf("recent[0].ID = %d, want 2", rec[0].ID)
		}
		if rec[1].ID != 1 {
			t.Errorf("recent[1].ID = %d, want 1", rec[1].ID)
		}
	}
}

func TestPingPluginLatestRecentEmpty(t *testing.T) {
	p, _ := core.ByName("ping")
	latest, recent, count := p.LatestRecent([]PingCheck{}, 15)
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history, got latest=%v, recent=%v, count=%d", latest, recent, count)
	}
}

func TestPingPluginLatestRecentSingle(t *testing.T) {
	p, _ := core.ByName("ping")
	history := []PingCheck{
		{ID: 1, Pass: core.PASS, Timestamp: "2024-01-01T00:00:00Z"},
	}
	latest, recent, count := p.LatestRecent(history, 15)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*PingCheck); !ok || l.ID != 1 {
		t.Errorf("latest.ID = want 1, got %d", l.ID)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for single entry", count)
	}
	// With a single entry, recent is an empty (non-nil) slice — make([]PingCheck, 0).
	if rec, ok := recent.([]PingCheck); ok {
		if len(rec) != 0 {
			t.Errorf("recent length = %d, want 0 for single entry", len(rec))
		}
	} else {
		t.Errorf("recent type assertion failed: recent=%v", recent)
	}
}

func TestPingPluginTemplateNames(t *testing.T) {
	p, _ := core.ByName("ping")
	row, body := p.TemplateNames()
	if row != "check_ping_row" {
		t.Errorf("row template = %q, want %q", row, "check_ping_row")
	}
	if body != "check_ping_body" {
		t.Errorf("body template = %q, want %q", body, "check_ping_body")
	}
}
