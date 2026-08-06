package core

import (
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/milochristiansen/lua"
)

// --- Mock -------------------------------------------------------------------

// testPlugin is a minimal CheckPlugin implementation for testing the registry.
type testPlugin struct {
	name      string
	tableName string
}

func (p *testPlugin) TypeName() string         { return p.name }
func (p *testPlugin) TableName() string        { return p.tableName }
func (p *testPlugin) CreateTableDDL() []string { return nil }
func (p *testPlugin) CreateIndexDDL() []string { return nil }
func (p *testPlugin) Insert(_ *sql.DB, _, _ string, _ int64, _ json.RawMessage) error {
	return nil
}
func (p *testPlugin) InsertError(_ *sql.DB, _, _ string, _ int64, _ int, _ string) error {
	return nil
}
func (p *testPlugin) QuerySince(_ *sql.DB, _, _ string, _ time.Time) (interface{}, error) {
	return nil, nil
}
func (p *testPlugin) ExtractPoints(_ interface{}) []CheckPoint         { return nil }
func (p *testPlugin) ExtractDurationPoints(_ interface{}) []CheckPoint { return nil }
func (p *testPlugin) LatestRecent(_ interface{}) (latest, recent interface{}, count int) {
	return nil, nil, 0
}
func (p *testPlugin) RegisterLua(_ *lua.State, _ int) { /* requires real Lua state — skip */ }
func (p *testPlugin) DispatchWireResult(_ ResourceMeta, _ CheckResult, _ time.Duration) WireResult {
	return WireResult{}
}
func (p *testPlugin) TemplateNames() (row, body string) { return "", "" }

// --- Tests ------------------------------------------------------------------

func TestRegister(t *testing.T) {
	p := &testPlugin{name: "test_register_type", tableName: "checks_test_register"}

	// Register and verify it appears.
	Register(p)

	got := All()
	found := false
	for _, plugin := range got {
		if plugin.TypeName() == "test_register_type" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("All() did not include the registered plugin")
	}

	ret, ok := ByName("test_register_type")
	if !ok {
		t.Fatal("ByName returned false after registration")
	}
	if ret != p {
		t.Errorf("ByName returned plugin %v, want %v", ret, p)
	}
}

func TestAllEmpty(t *testing.T) {
	// All() must never return nil, even if no plugins have been registered.
	// Note: real init() functions from imported check types may have already
	// populated the registry; this test only asserts the slice is non-nil.
	plugins := All()
	if plugins == nil {
		t.Error("All() returned nil, want non-nil (possibly empty) slice")
	}
}

func TestByNameMissing(t *testing.T) {
	_, ok := ByName("nonexistent_check_type_xyz")
	if ok {
		t.Error("ByName returned true for a nonexistent type, want false")
	}
}

func TestConcurrentAccess(t *testing.T) {
	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				// Alternate between All and ByName to exercise both read paths.
				if (id+j)%2 == 0 {
					_ = All()
				} else {
					_, _ = ByName("http")
					_, _ = ByName("nonexistent")
				}
			}
		}(i)
	}

	wg.Wait()
	// If the race detector is enabled, it will catch any unsynchronized access.
	// No panics means the test passes.
}
