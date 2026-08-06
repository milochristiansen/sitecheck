package outpost

import (
	"encoding/json"
	"testing"
	"time"

	"sitecheck/core"
)

func TestOutpostResultInterface(t *testing.T) {
	r := &OutpostResult{
		Pass:           core.PASS,
		FailReason:     "",
		ResponseTimeMS: 42.5,
		Error:          "",
		CheckCount:     5,
		FailCount:      0,
	}

	if r.CheckType() != "outpost" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "outpost")
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

func TestOutpostPluginRegistration(t *testing.T) {
	p, ok := core.ByName("outpost")
	if !ok {
		t.Fatal("Outpost plugin not registered -- did init() run?")
	}
	if p.TypeName() != "outpost" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "outpost")
	}
	if p.TableName() != "checks_outpost" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_outpost")
	}
}

func TestOutpostPluginCreateTableDDL(t *testing.T) {
	p, _ := core.ByName("outpost")
	ddl := p.CreateTableDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateTableDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestOutpostPluginCreateIndexDDL(t *testing.T) {
	p, _ := core.ByName("outpost")
	ddl := p.CreateIndexDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateIndexDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

func TestOutpostPluginDispatchWireResult(t *testing.T) {
	p, _ := core.ByName("outpost")
	meta := core.ResourceMeta{
		Slug:           "test-outpost",
		Name:           "Test Outpost",
		Desc:           "An outpost test",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &OutpostResult{
		Pass:           core.DEGRADED,
		FailReason:     "partial failure",
		ResponseTimeMS: 200.0,
		Error:          "",
		CheckCount:     10,
		FailCount:      3,
	}
	elapsed := 500 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-outpost" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-outpost")
	}
	if wr.CheckType != "outpost" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "outpost")
	}
	if wr.Pass != 0 {
		t.Errorf("Pass = %d, want 0 (ignored by no-op)", wr.Pass)
	}
	if wr.FailReason != "" {
		t.Errorf("FailReason = %q, want empty", wr.FailReason)
	}
	if wr.ResponseMS != 0 {
		t.Errorf("ResponseMS = %f, want 0 (ignored by no-op)", wr.ResponseMS)
	}
	if wr.Error != "outpost results are synthesized by the core" {
		t.Errorf("Error = %q, want %q", wr.Error, "outpost results are synthesized by the core")
	}
	if wr.ElapsedMS != 500 {
		t.Errorf("ElapsedMS = %d, want %d", wr.ElapsedMS, 500)
	}
	if wr.NotifyPass != true {
		t.Errorf("NotifyPass = %v, want true", wr.NotifyPass)
	}
	if wr.NotifyFail != true {
		t.Errorf("NotifyFail = %v, want true", wr.NotifyFail)
	}
	if wr.Data != nil {
		t.Errorf("Data = %v, want nil", string(wr.Data))
	}
}

func TestOutpostPluginExtractPoints(t *testing.T) {
	p, _ := core.ByName("outpost")
	history := []OutpostCheck{
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

func TestOutpostPluginExtractPointsNil(t *testing.T) {
	p, _ := core.ByName("outpost")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

func TestOutpostPluginExtractDurationPoints(t *testing.T) {
	p, _ := core.ByName("outpost")
	history := []OutpostCheck{
		{DurationMS: 100, Pass: core.PASS, Timestamp: "2024-01-01T00:00:00Z"},
		{DurationMS: 200, Pass: core.FAIL, Timestamp: "2024-01-01T00:01:00Z"},
	}
	pts := p.ExtractDurationPoints(history)
	if len(pts) != 2 {
		t.Fatalf("ExtractDurationPoints = %d points, want 2", len(pts))
	}
	if pts[0].Resp != 100.0 {
		t.Errorf("pts[0].Resp = %f, want 100.0", pts[0].Resp)
	}
	if pts[0].Pass != core.PASS {
		t.Errorf("pts[0].Pass = %d, want %d", pts[0].Pass, core.PASS)
	}
	if pts[1].Resp != 200.0 {
		t.Errorf("pts[1].Resp = %f, want 200.0", pts[1].Resp)
	}
}

func TestOutpostPluginExtractDurationPointsNil(t *testing.T) {
	p, _ := core.ByName("outpost")
	pts := p.ExtractDurationPoints(nil)
	if pts != nil {
		t.Errorf("ExtractDurationPoints(nil) = %v, want nil", pts)
	}
}

func TestOutpostPluginLatestRecentMulti(t *testing.T) {
	p, _ := core.ByName("outpost")
	history := []OutpostCheck{
		{ID: 1, Pass: core.FAIL, ResponseTimeMS: 5.0},
		{ID: 2, Pass: core.PASS, ResponseTimeMS: 15.0},
		{ID: 3, Pass: core.DEGRADED, ResponseTimeMS: 10.0},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*OutpostCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3, got %d", l.ID)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]OutpostCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = want 2, got %d", len(rec))
	}
}

func TestOutpostPluginLatestRecentEmpty(t *testing.T) {
	p, _ := core.ByName("outpost")
	latest, recent, count := p.LatestRecent([]OutpostCheck{})
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history")
	}
}

func TestOutpostPluginLatestRecentSingle(t *testing.T) {
	p, _ := core.ByName("outpost")
	history := []OutpostCheck{
		{ID: 42, Pass: core.PASS, ResponseTimeMS: 7.5},
	}
	latest, recent, count := p.LatestRecent(history)
	if latest == nil {
		t.Fatal("latest is nil for single entry")
	}
	if l, ok := latest.(*OutpostCheck); !ok || l.ID != 42 {
		t.Errorf("latest.ID = want 42, got %d", l.ID)
	}
	// With only 1 entry, latest is returned but recent is nil.
	if count != 0 {
		t.Errorf("count = %d, want 0 for single entry (0 recent)", count)
	}
	if recent != nil {
		t.Errorf("recent = %v, want nil for single entry", recent)
	}
}

func TestOutpostPluginTemplateNames(t *testing.T) {
	p, _ := core.ByName("outpost")
	row, body := p.TemplateNames()
	if row != "check_outpost_row" {
		t.Errorf("row template = %q, want %q", row, "check_outpost_row")
	}
	if body != "check_outpost_body" {
		t.Errorf("body template = %q, want %q", body, "check_outpost_body")
	}
}

func TestOutpostPluginRegisterLua(t *testing.T) {
	// RegisterLua is a documented no-op — just verify it doesn't panic.
	p, _ := core.ByName("outpost")
	p.RegisterLua(nil, 30)
}

func TestOutpostResultCheckResponseMS(t *testing.T) {
	r := &OutpostResult{
		Pass:           core.FAIL,
		FailReason:     "timeout",
		ResponseTimeMS: 0,
	}
	if r.CheckResponseMS() != 0.0 {
		t.Errorf("CheckResponseMS = %f, want 0.0", r.CheckResponseMS())
	}
}

func TestOutpostCheckFields(t *testing.T) {
	ch := OutpostCheck{
		ID:             123,
		Slug:           "test-slug",
		OutpostSlug:    "outpost-1",
		Timestamp:      "2024-06-15T12:00:00Z",
		DurationMS:     250,
		Pass:           core.PASS,
		ResponseTimeMS: 125.0,
		CheckCount:     10,
		FailCount:      2,
		Error:          "",
	}
	if ch.ID != 123 {
		t.Errorf("ID = %d, want 123", ch.ID)
	}
	if ch.Slug != "test-slug" {
		t.Errorf("Slug = %q, want %q", ch.Slug, "test-slug")
	}
	if ch.OutpostSlug != "outpost-1" {
		t.Errorf("OutpostSlug = %q, want %q", ch.OutpostSlug, "outpost-1")
	}
	if ch.DurationMS != 250 {
		t.Errorf("DurationMS = %d, want 250", ch.DurationMS)
	}
	if ch.ResponseTimeMS != 125.0 {
		t.Errorf("ResponseTimeMS = %f, want 125.0", ch.ResponseTimeMS)
	}
	if ch.CheckCount != 10 {
		t.Errorf("CheckCount = %d, want 10", ch.CheckCount)
	}
	if ch.FailCount != 2 {
		t.Errorf("FailCount = %d, want 2", ch.FailCount)
	}
}

func TestOutpostResultJSON(t *testing.T) {
	r := OutpostResult{
		Pass:           core.FAIL,
		FailReason:     "connection refused",
		ResponseTimeMS: 0.5,
		Error:          "dial tcp: connection refused",
		CheckCount:     3,
		FailCount:      3,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal OutpostResult: %v", err)
	}

	var r2 OutpostResult
	if err := json.Unmarshal(data, &r2); err != nil {
		t.Fatalf("Unmarshal OutpostResult: %v", err)
	}
	if r2.Pass != core.FAIL {
		t.Errorf("Pass = %d, want %d", r2.Pass, core.FAIL)
	}
	if r2.FailReason != "connection refused" {
		t.Errorf("FailReason = %q, want %q", r2.FailReason, "connection refused")
	}
	if r2.ResponseTimeMS != 0.5 {
		t.Errorf("ResponseTimeMS = %f, want 0.5", r2.ResponseTimeMS)
	}
	if r2.Error != "dial tcp: connection refused" {
		t.Errorf("Error = %q, want %q", r2.Error, "dial tcp: connection refused")
	}
}
