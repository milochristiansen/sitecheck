// Package systemd implements core.CheckPlugin for systemd service checks.
package systemd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/milochristiansen/lua"

	"sitecheck/core"
)

// --- Result struct -----------------------------------------------------------

// SystemdResult implements core.CheckResult for systemd service checks.
type SystemdResult struct {
	Pass           int
	FailReason     string
	ServiceName    string
	ActiveState    string
	SubState       string
	LoadState      string
	MainPID        int
	ResponseTimeMS float64
	Error          string
}

func (r *SystemdResult) CheckType() string        { return "systemd" }
func (r *SystemdResult) CheckPass() int           { return r.Pass }
func (r *SystemdResult) CheckFailReason() string  { return r.FailReason }
func (r *SystemdResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// --- DB row struct -----------------------------------------------------------

type SystemdCheck struct {
	ID             int64
	Slug           string
	OutpostSlug    string
	Timestamp      string
	DurationMS     int64
	Pass           int
	ResponseTimeMS float64
	ServiceName    string
	ActiveState    string
	SubState       string
	LoadState      string
	MainPID        int
	Error          string
}

// --- Plugin ------------------------------------------------------------------

type impl struct{}

func (p *impl) TypeName() string { return "systemd" }

func (p *impl) TableName() string { return "checks_systemd" }

func (p *impl) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_systemd (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slug TEXT NOT NULL,
			outpost_slug TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			pass INTEGER NOT NULL DEFAULT 0,
			response_time_ms REAL NOT NULL DEFAULT 0,
			service_name TEXT NOT NULL,
			active_state TEXT NOT NULL DEFAULT '',
			sub_state TEXT NOT NULL DEFAULT '',
			load_state TEXT NOT NULL DEFAULT '',
			main_pid INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT ''
		)`,
	}
}

func (p *impl) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_systemd_slug ON checks_systemd(slug)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_systemd_outpost_slug ON checks_systemd(outpost_slug)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_systemd_timestamp ON checks_systemd(timestamp)`,
	}
}

// --- DB operations -----------------------------------------------------------

func (p *impl) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r SystemdResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal systemd result: %w", err)
	}
	c := SystemdCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		ServiceName:    r.ServiceName,
		ActiveState:    r.ActiveState,
		SubState:       r.SubState,
		LoadState:      r.LoadState,
		MainPID:        r.MainPID,
		Error:          r.Error,
	}
	_, err := db.Exec(
		`INSERT INTO checks_systemd
			(slug, outpost_slug, duration_ms, pass, response_time_ms, service_name, active_state, sub_state, load_state, main_pid, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Slug, c.OutpostSlug, c.DurationMS, c.Pass, c.ResponseTimeMS,
		c.ServiceName, c.ActiveState, c.SubState, c.LoadState, c.MainPID, c.Error,
	)
	if err != nil {
		return fmt.Errorf("insert systemd check: %w", err)
	}
	return nil
}

func (p *impl) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_systemd
			(slug, outpost_slug, duration_ms, pass, error, service_name)
		VALUES (?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, errMsg, "(error)",
	)
	if err != nil {
		return fmt.Errorf("insert systemd check error: %w", err)
	}
	return nil
}

func (p *impl) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			service_name, active_state, sub_state, load_state, main_pid, error
		FROM checks_systemd WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query systemd checks since: %w", err)
	}
	defer rows.Close()

	var checks []SystemdCheck
	for rows.Next() {
		var (
			c              SystemdCheck
			durationMS     sql.NullInt64
			responseTimeMS sql.NullFloat64
			serviceName    sql.NullString
			activeState    sql.NullString
			subState       sql.NullString
			loadState      sql.NullString
			mainPID        sql.NullInt64
			errMsg         sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseTimeMS, &serviceName, &activeState, &subState,
			&loadState, &mainPID, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan systemd check since: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseTimeMS.Float64
		c.ServiceName = serviceName.String
		c.ActiveState = activeState.String
		c.SubState = subState.String
		c.LoadState = loadState.String
		c.MainPID = int(mainPID.Int64)
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// --- Common field access -----------------------------------------------------

func (p *impl) ExtractPoints(history interface{}) []core.CheckPoint {
	h, ok := history.([]SystemdCheck)
	if !ok {
		return nil
	}
	pts := make([]core.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = core.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (p *impl) ExtractDurationPoints(history interface{}) []core.CheckPoint {
	return nil
}

func (p *impl) LatestRecent(history interface{}) (latest, recent interface{}, count int) {
	h, ok := history.([]SystemdCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	n := len(h) - 1
	recentSlice := make([]SystemdCheck, n)
	for i := range n {
		recentSlice[i] = h[len(h)-2-i]
	}
	return latest, recentSlice, n
}

// --- Lua registration --------------------------------------------------------

var (
	systemdConn     *systemdDbus.Conn
	systemdConnOnce sync.Once
	systemdConnErr  error
)

func getSystemdConn() (*systemdDbus.Conn, error) {
	systemdConnOnce.Do(func() {
		systemdConn, systemdConnErr = systemdDbus.New()
	})
	return systemdConn, systemdConnErr
}

func dbusValueString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func dbusValueInt(v interface{}) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case uint32:
		return int(n)
	case int64:
		return int(n)
	case uint64:
		return int(n)
	}
	return 0
}

func pushSystemdResult(l *lua.State, r *SystemdResult) {
	core.PushResultUserData(l, r, map[string]core.LuaField{
		"Pass":           {Get: func(l *lua.State) { l.Push(int64(r.Pass)) }, Set: func(l *lua.State) { r.Pass = int(l.ToInt(3)) }},
		"FailReason":     {Get: func(l *lua.State) { l.Push(r.FailReason) }, Set: func(l *lua.State) { r.FailReason = l.ToString(3) }},
		"ServiceName":    {Get: func(l *lua.State) { l.Push(r.ServiceName) }, Set: func(l *lua.State) { r.ServiceName = l.ToString(3) }},
		"ActiveState":    {Get: func(l *lua.State) { l.Push(r.ActiveState) }, Set: func(l *lua.State) { r.ActiveState = l.ToString(3) }},
		"SubState":       {Get: func(l *lua.State) { l.Push(r.SubState) }, Set: func(l *lua.State) { r.SubState = l.ToString(3) }},
		"LoadState":      {Get: func(l *lua.State) { l.Push(r.LoadState) }, Set: func(l *lua.State) { r.LoadState = l.ToString(3) }},
		"MainPID":        {Get: func(l *lua.State) { l.Push(int64(r.MainPID)) }},
		"ResponseTimeMS": {Get: func(l *lua.State) { l.Push(r.ResponseTimeMS) }},
		"Error":          {Get: func(l *lua.State) { l.Push(r.Error) }, Set: func(l *lua.State) { r.Error = l.ToString(3) }},
	})
}

func (p *impl) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		serviceName := l.ToString(1)

		timeout := defaultTimeout
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			timeout = core.ReadIntOpt(l, 2, "timeout", defaultTimeout)
		}

		r := &SystemdResult{
			Pass:        core.FAIL,
			ServiceName: serviceName,
		}

		conn, err := getSystemdConn()
		if err != nil {
			r.Error = "systemd dbus connection failed: " + err.Error()
			pushSystemdResult(l, r)
			return 1
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		start := time.Now()

		// Query unit properties via D-Bus.
		props, err := conn.GetUnitPropertiesContext(ctx, serviceName)
		elapsed := time.Since(start)
		r.ResponseTimeMS = elapsed.Seconds() * 1000

		if err != nil {
			r.Error = err.Error()
			pushSystemdResult(l, r)
			return 1
		}

		if v, ok := props["ActiveState"]; ok {
			r.ActiveState = dbusValueString(v)
		}
		if v, ok := props["SubState"]; ok {
			r.SubState = dbusValueString(v)
		}
		if v, ok := props["LoadState"]; ok {
			r.LoadState = dbusValueString(v)
		}
		if v, ok := props["MainPID"]; ok {
			r.MainPID = dbusValueInt(v)
		}

		pushSystemdResult(l, r)
		return 1
	})
	l.SetGlobal("systemd_check")
}

// --- Wire dispatch -----------------------------------------------------------

func (p *impl) DispatchWireResult(res core.ResourceMeta, cr core.CheckResult, elapsed time.Duration) core.WireResult {
	r := cr.(*SystemdResult)
	return core.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"systemd", r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error, r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

// --- Templates ---------------------------------------------------------------

func (p *impl) TemplateNames() (string, string) {
	return "check_systemd_row", "check_systemd_body"
}

// --- Registration ------------------------------------------------------------

func init() {
	core.Register(&impl{})
}
