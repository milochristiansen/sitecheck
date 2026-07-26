package systemd

import (
	"encoding/json"
	"testing"
	"time"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// --- Result interface ---------------------------------------------------------

func TestSystemdResultInterface(t *testing.T) {
	r := &SystemdResult{
		Pass:           protocol.PASS,
		FailReason:     "",
		ResponseTimeMS: 12.5,
		ServiceName:    "nginx.service",
		ActiveState:    "active",
		SubState:       "running",
	}

	if r.CheckType() != "systemd" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "systemd")
	}
	if r.CheckPass() != protocol.PASS {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), protocol.PASS)
	}
	if r.CheckFailReason() != "" {
		t.Errorf("CheckFailReason = %q, want empty", r.CheckFailReason())
	}
	if r.CheckResponseMS() != 12.5 {
		t.Errorf("CheckResponseMS = %f, want %f", r.CheckResponseMS(), 12.5)
	}
}

// --- Plugin registration -----------------------------------------------------

func TestSystemdPluginRegistration(t *testing.T) {
	p, ok := registry.ByName("systemd")
	if !ok {
		t.Fatal("systemd plugin not registered — did init() run?")
	}
	if p.TypeName() != "systemd" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "systemd")
	}
	if p.TableName() != "checks_systemd" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_systemd")
	}
}

// --- DDL --------------------------------------------------------------------

func TestSystemdPluginCreateTableDDL(t *testing.T) {
	p, _ := registry.ByName("systemd")
	ddl := p.CreateTableDDL()
	if len(ddl) != 1 {
		t.Fatalf("CreateTableDDL returned %d statements, want 1", len(ddl))
	}
	if ddl[0][:13] != "CREATE TABLE " {
		t.Errorf("First DDL is not a CREATE TABLE: %s", ddl[0][:50])
	}
}

func TestSystemdPluginCreateIndexDDL(t *testing.T) {
	p, _ := registry.ByName("systemd")
	ddl := p.CreateIndexDDL()
	if len(ddl) != 3 {
		t.Fatalf("CreateIndexDDL returned %d statements, want 3", len(ddl))
	}
	if ddl[0][:13] != "CREATE INDEX " {
		t.Errorf("First DDL is not a CREATE INDEX: %s", ddl[0][:50])
	}
}

// --- DispatchWireResult -----------------------------------------------------

func TestSystemdPluginDispatchWireResult(t *testing.T) {
	p, _ := registry.ByName("systemd")
	meta := registry.ResourceMeta{
		Slug:           "test-systemd",
		Name:           "Test Systemd",
		Desc:           "A test",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &SystemdResult{
		Pass:           protocol.DEGRADED,
		FailReason:     "service inactive",
		ServiceName:    "nginx.service",
		ActiveState:    "inactive",
		SubState:       "dead",
		LoadState:      "loaded",
		MainPID:        0,
		ResponseTimeMS: 25.0,
		Error:          "",
	}
	elapsed := 50 * time.Millisecond

	wr := p.DispatchWireResult(meta, cr, elapsed)

	if wr.Slug != "test-systemd" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-systemd")
	}
	if wr.CheckType != "systemd" {
		t.Errorf("CheckType = %q, want %q", wr.CheckType, "systemd")
	}
	if wr.Pass != protocol.DEGRADED {
		t.Errorf("Pass = %d, want %d", wr.Pass, protocol.DEGRADED)
	}
	if wr.FailReason != "service inactive" {
		t.Errorf("FailReason = %q, want %q", wr.FailReason, "service inactive")
	}
	if wr.ResponseMS != 25.0 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 25.0)
	}
	if wr.ElapsedMS != 50 {
		t.Errorf("ElapsedMS = %d, want %d", wr.ElapsedMS, 50)
	}

	// Verify Data contains the ServiceName field.
	var result SystemdResult
	if err := json.Unmarshal(wr.Data, &result); err != nil {
		t.Fatalf("Data unmarshal: %v", err)
	}
	if result.ServiceName != "nginx.service" {
		t.Errorf("Data.ServiceName = %q, want %q", result.ServiceName, "nginx.service")
	}
	if result.ActiveState != "inactive" {
		t.Errorf("Data.ActiveState = %q, want %q", result.ActiveState, "inactive")
	}
}

// --- ExtractPoints ----------------------------------------------------------

func TestSystemdPluginExtractPoints(t *testing.T) {
	p, _ := registry.ByName("systemd")
	history := []SystemdCheck{
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

func TestSystemdPluginExtractPointsNil(t *testing.T) {
	p, _ := registry.ByName("systemd")
	pts := p.ExtractPoints(nil)
	if pts != nil {
		t.Errorf("ExtractPoints(nil) = %v, want nil", pts)
	}
}

// --- ExtractDurationPoints ---------------------------------------------------

func TestSystemdPluginExtractDurationPoints(t *testing.T) {
	p, _ := registry.ByName("systemd")
	pts := p.ExtractDurationPoints([]SystemdCheck{})
	if pts != nil {
		t.Errorf("ExtractDurationPoints should return nil for systemd")
	}
}

// --- LatestRecent ------------------------------------------------------------

func TestSystemdPluginLatestRecent(t *testing.T) {
	p, _ := registry.ByName("systemd")
	history := []SystemdCheck{
		{ID: 1, Pass: protocol.FAIL, ServiceName: "sshd.service"},
		{ID: 2, Pass: protocol.PASS, ServiceName: "sshd.service"},
		{ID: 3, Pass: protocol.DEGRADED, ServiceName: "sshd.service"},
	}
	latest, recent, count := p.LatestRecent(history, 15)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*SystemdCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3, got %d", func() int64 {
			if l, ok := latest.(*SystemdCheck); ok {
				return l.ID
			}
			return -1
		}())
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if rec, ok := recent.([]SystemdCheck); !ok || len(rec) != 2 {
		t.Errorf("recent length = want 2, got %d", len(rec))
	}
}

func TestSystemdPluginLatestRecentEmpty(t *testing.T) {
	p, _ := registry.ByName("systemd")
	latest, recent, count := p.LatestRecent([]SystemdCheck{}, 15)
	if latest != nil || recent != nil || count != 0 {
		t.Errorf("Expected nil,nil,0 for empty history")
	}
}

func TestSystemdPluginLatestRecentSingle(t *testing.T) {
	p, _ := registry.ByName("systemd")
	history := []SystemdCheck{
		{ID: 42, Pass: protocol.PASS, ServiceName: "nginx.service"},
	}
	latest, recent, count := p.LatestRecent(history, 15)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*SystemdCheck); !ok || l.ID != 42 {
		t.Errorf("latest.ID = want 42, got %d", func() int64 {
			if l, ok := latest.(*SystemdCheck); ok {
				return l.ID
			}
			return -1
		}())
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (only one entry, nothing to be 'recent')", count)
	}
	if rec, ok := recent.([]SystemdCheck); !ok || len(rec) != 0 {
		t.Errorf("recent length = want 0, got %d", len(rec))
	}
}

func TestSystemdPluginLatestRecentBounded(t *testing.T) {
	p, _ := registry.ByName("systemd")
	history := []SystemdCheck{
		{ID: 1, Pass: protocol.FAIL},
		{ID: 2, Pass: protocol.PASS},
		{ID: 3, Pass: protocol.DEGRADED},
	}
	// maxRecent=1 should limit the recent slice to 1 element.
	latest, recent, count := p.LatestRecent(history, 1)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	if l, ok := latest.(*SystemdCheck); !ok || l.ID != 3 {
		t.Errorf("latest.ID = want 3, got %d", func() int64 {
			if l, ok := latest.(*SystemdCheck); ok {
				return l.ID
			}
			return -1
		}())
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (bounded by maxRecent=1)", count)
	}
	if rec, ok := recent.([]SystemdCheck); !ok || len(rec) != 1 {
		t.Errorf("recent length = want 1, got %d", len(rec))
	} else if rec[0].ID != 2 {
		t.Errorf("recent[0].ID = %d, want 2", rec[0].ID)
	}
}

// --- Templates --------------------------------------------------------------

func TestSystemdPluginTemplateNames(t *testing.T) {
	p, _ := registry.ByName("systemd")
	row, body := p.TemplateNames()
	if row != "check_systemd_row" {
		t.Errorf("row template = %q, want %q", row, "check_systemd_row")
	}
	if body != "check_systemd_body" {
		t.Errorf("body template = %q, want %q", body, "check_systemd_body")
	}
}

func TestSystemdPluginTemplateFiles(t *testing.T) {
	p, _ := registry.ByName("systemd")
	files := p.TemplateFiles()
	if len(files) != 1 {
		t.Fatalf("TemplateFiles returned %d files, want 1", len(files))
	}
	if files[0] != "templates/checks/systemd.html" {
		t.Errorf("TemplateFiles[0] = %q, want %q", files[0], "templates/checks/systemd.html")
	}
}

// --- dbusValueString --------------------------------------------------------

func TestDbusValueString(t *testing.T) {
	tests := []struct {
		input interface{}
		want  string
	}{
		{input: "active", want: "active"},
		{input: "", want: ""},
		{input: int32(42), want: ""},
		{input: nil, want: ""},
	}
	for _, tt := range tests {
		got := dbusValueString(tt.input)
		if got != tt.want {
			t.Errorf("dbusValueString(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- dbusValueInt -----------------------------------------------------------

func TestDbusValueInt(t *testing.T) {
	tests := []struct {
		input interface{}
		want  int
	}{
		{input: int32(42), want: 42},
		{input: uint32(42), want: 42},
		{input: int64(42), want: 42},
		{input: uint64(42), want: 42},
		{input: int32(-1), want: -1},
		{input: uint32(0), want: 0},
		{input: uint64(9999999999), want: 9999999999},
		{input: "hello", want: 0},
		{input: nil, want: 0},
		{input: true, want: 0},
	}
	for _, tt := range tests {
		got := dbusValueInt(tt.input)
		if got != tt.want {
			t.Errorf("dbusValueInt(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
