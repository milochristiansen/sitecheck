package core

import (
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/milochristiansen/lua"
)

// CheckPoint is a common data point used by sparklines, line charts, and uptime
// calculations. It extracts the fields shared by all check types.
type CheckPoint struct {
	Pass int
	Resp float64
	TS   string
}

// ResourceMeta carries the metadata fields from a resource that DispatchWireResult
// needs to build a WireResult.
type ResourceMeta struct {
	Slug           string
	Name           string
	Desc           string
	NotifyPass     bool
	NotifyDegraded bool
	NotifyFail     bool
}

// CheckPlugin is the interface every check type must implement.
// A single concrete type implements all methods; both the core (sitecheck) and
// the scoutpost binary use the subset relevant to them.
type CheckPlugin interface {
	// Identity.
	TypeName() string // e.g. "http", "ping", "tcp", "dns", "ssl", "systemd", "outpost"

	// DB schema.
	TableName() string
	CreateTableDDL() []string
	CreateIndexDDL() []string

	// DB read/write.
	Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error
	InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error
	QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error)

	// Common field access for sparklines, charts, and stats.
	ExtractPoints(history interface{}) []CheckPoint
	ExtractDurationPoints(history interface{}) []CheckPoint
	LatestRecent(history interface{}) (latest, recent interface{}, count int)

	// Scoutpost: Lua registration.
	RegisterLua(l *lua.State, defaultTimeout int)

	// Scoutpost: wire dispatch from Lua result to WireResult.
	DispatchWireResult(res ResourceMeta, cr CheckResult, elapsed time.Duration) WireResult

	// Templates.
	TemplateNames() (row, body string)
}

var (
	mu      sync.RWMutex
	plugins []CheckPlugin
	byName  = map[string]CheckPlugin{}
)

// Register adds a plugin to the global registry. Must be called from init().
func Register(p CheckPlugin) {
	mu.Lock()
	defer mu.Unlock()
	plugins = append(plugins, p)
	byName[p.TypeName()] = p
}

// All returns a snapshot of all registered plugins in registration order.
func All() []CheckPlugin {
	mu.RLock()
	defer mu.RUnlock()
	return append([]CheckPlugin(nil), plugins...)
}

// ByName returns the plugin for the given check type name, or false if not found.
func ByName(name string) (CheckPlugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byName[name]
	return p, ok
}
