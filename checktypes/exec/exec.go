// Package exec implements registry.CheckPlugin for arbitrary command-execution checks.
package exec

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/milochristiansen/lua"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// maxOutputBytes is the per-field truncation limit for stdout, stderr, and
// combined when stored in the database or serialized on the wire.
const maxOutputBytes = 64 << 10 // 64 KiB

// --- Result struct ----------------------------------------------------------

// ExecResult is the check-type-specific result for a command execution.
// It implements protocol.CheckResult.
type ExecResult struct {
	Pass           int     `json:"pass"`
	FailReason     string  `json:"fail_reason"`
	ResponseTimeMS float64 `json:"response_time_ms"`
	Command        string  `json:"command"`
	ExitCode       int     `json:"exit_code"`
	Stdout         string  `json:"stdout"`
	Stderr         string  `json:"stderr"`
	Combined       string  `json:"combined"`
	Error          string  `json:"error"`
}

func (r *ExecResult) CheckType() string        { return "exec" }
func (r *ExecResult) CheckPass() int            { return r.Pass }
func (r *ExecResult) CheckFailReason() string   { return r.FailReason }
func (r *ExecResult) CheckResponseMS() float64  { return r.ResponseTimeMS }

// --- DB row struct ----------------------------------------------------------

// ExecCheck is a single row from checks_exec.
type ExecCheck struct {
	ID             string  `json:"id"`
	Slug           string  `json:"slug"`
	Timestamp      string  `json:"timestamp"`
	DurationMS     int64   `json:"duration_ms"`
	Pass           int     `json:"pass"`
	ResponseTimeMS float64 `json:"response_time_ms"`
	Command        string  `json:"command"`
	ExitCode       int     `json:"exit_code"`
	Stdout         string  `json:"stdout"`
	Stderr         string  `json:"stderr"`
	Combined       string  `json:"combined"`
	Error          string  `json:"error"`
}

// --- Plugin -----------------------------------------------------------------

type plugin struct{}

// --- Identity ---------------------------------------------------------------

func (p *plugin) TypeName() string { return "exec" }

// --- DB schema --------------------------------------------------------------

func (p *plugin) TableName() string { return "checks_exec" }

func (p *plugin) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_exec (
			id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			slug              TEXT    NOT NULL,
			outpost_slug      TEXT    NOT NULL,
			timestamp         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
			duration_ms       INTEGER,
			pass              INTEGER NOT NULL,
			response_time_ms  REAL,
			command           TEXT,
			exit_code         INTEGER,
			stdout            TEXT,
			stderr            TEXT,
			combined          TEXT,
			error             TEXT
		)`,
	}
}

func (p *plugin) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_exec_slug ON checks_exec(slug, outpost_slug, timestamp)`,
	}
}

// --- DB operations ----------------------------------------------------------

func (p *plugin) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r ExecResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal exec result: %w", err)
	}

	_, err := db.Exec(
		`INSERT INTO checks_exec
			(slug, outpost_slug, duration_ms, pass, response_time_ms,
			 command, exit_code, stdout, stderr, combined, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.Command, r.ExitCode, truncate(r.Stdout), truncate(r.Stderr), truncate(r.Combined), r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert exec check: %w", err)
	}
	return nil
}

func (p *plugin) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_exec
			(slug, outpost_slug, duration_ms, pass, error)
		VALUES (?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert exec error: %w", err)
	}
	return nil
}

func (p *plugin) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			command, exit_code, stdout, stderr, combined, error
		FROM checks_exec WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query exec checks since: %w", err)
	}
	defer rows.Close()

	var checks []ExecCheck
	for rows.Next() {
		var (
			c          ExecCheck
			durationMS sql.NullInt64
			responseMS sql.NullFloat64
			command    sql.NullString
			exitCode   sql.NullInt64
			stdout     sql.NullString
			stderr     sql.NullString
			combined   sql.NullString
			errMsg     sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &command, &exitCode, &stdout, &stderr, &combined, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan exec check: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Command = command.String
		c.ExitCode = int(exitCode.Int64)
		c.Stdout = stdout.String
		c.Stderr = stderr.String
		c.Combined = combined.String
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// --- Common field access ----------------------------------------------------

func (p *plugin) ExtractPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]ExecCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = registry.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (p *plugin) ExtractDurationPoints(_ interface{}) []registry.CheckPoint {
	return nil
}

func (p *plugin) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]ExecCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	if len(h) == 1 {
		return latest, nil, 0
	}
	n := len(h) - 1
	if n > maxRecent {
		n = maxRecent
	}
	rec := make([]ExecCheck, n)
	for i := range n {
		rec[i] = h[len(h)-2-i]
	}
	return latest, rec, n
}

// --- Lua registration -------------------------------------------------------

func (p *plugin) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		name := l.ToString(1)

		// Collect positional args from the second argument (1-indexed Lua table).
		var args []string
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			args = readStrSlice(l, 2)
		}

		// Options table (third argument).
		timeout := defaultTimeout
		var env map[string]string
		var stdin string
		if !l.IsNil(3) && l.TypeOf(3) == lua.TypTable {
			timeout = readIntOpt(l, 3, "timeout", defaultTimeout)
			env = readStringMapOpt(l, 3, "env")
			stdin = readStringOpt(l, 3, "stdin", "")
		}

		r := &ExecResult{
			Pass:    protocol.FAIL,
			Command: formatCommand(name, args),
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, name, args...)

		// Set environment if provided.
		if len(env) > 0 {
			cmd.Env = osEnviron()
			for k, v := range env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}

		// Wire up stdout, stderr, and combined capture with correct interleaving.
		var stdoutBuf, stderrBuf bytes.Buffer
		combWriter := &lockedWriter{w: new(bytes.Buffer)}
		cmd.Stdout = io.MultiWriter(&stdoutBuf, combWriter)
		cmd.Stderr = io.MultiWriter(&stderrBuf, combWriter)

		if stdin != "" {
			cmd.Stdin = bytes.NewReader([]byte(stdin))
		}

		start := time.Now()
		runErr := cmd.Run()
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		r.Stdout = stdoutBuf.String()
		r.Stderr = stderrBuf.String()
		r.Combined = combWriter.w.(*bytes.Buffer).String()

		if runErr != nil {
			// Distinguish timeout/context errors from non-zero exit.
			if ctx.Err() != nil {
				r.Error = fmt.Sprintf("command timed out after %ds", timeout)
			} else if ee, ok := runErr.(*exec.ExitError); ok {
				r.ExitCode = ee.ExitCode()
			} else {
				r.Error = runErr.Error()
			}
		}

		pushExecResult(l, r)
		return 1
	})
	l.SetGlobal("exec_command")
}

// --- Wire dispatch -----------------------------------------------------------

func (p *plugin) DispatchWireResult(res registry.ResourceMeta, cr protocol.CheckResult, elapsed time.Duration) protocol.WireResult {
	r := cr.(*ExecResult)
	return protocol.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"exec", r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error,
		r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

// --- Templates ---------------------------------------------------------------

func (p *plugin) TemplateNames() (string, string) {
	return "check_exec_row", "check_exec_body"
}

func (p *plugin) TemplateFiles() []string {
	return []string{"templates/checks/exec.html"}
}

// --- Registration ------------------------------------------------------------

func init() {
	registry.Register(&plugin{})
}

// --- Helpers -----------------------------------------------------------------

// lockedWriter wraps an io.Writer with a mutex so it is safe for concurrent use
// by the two goroutines os/exec uses to copy stdout and stderr.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// truncate returns s, truncated to maxOutputBytes. A suffix marker is appended
// when truncation occurs.
func truncate(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return s[:maxOutputBytes] + "\n…[truncated]"
}

// formatCommand joins name and args into a single display string, shell-quoting
// arguments that contain whitespace.
func formatCommand(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	b := &bytes.Buffer{}
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(' ')
		if needsQuote(a) {
			fmt.Fprintf(b, "%q", a)
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}

func needsQuote(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '"', '\'', '\\', '|', '&', ';', '<', '>', '(', ')', '$', '`', '*', '?', '[', ']', '{', '}', '!', '#', '~':
			return true
		}
	}
	return false
}

func osEnviron() []string { return os.Environ() }

// --- Lua helpers (mirrors of lmods unexported helpers) -----------------------

func pushExecResult(l *lua.State, r *ExecResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*ExecResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "Command":
			l.Push(r.Command)
		case "ExitCode":
			l.Push(int64(r.ExitCode))
		case "Stdout":
			l.Push(r.Stdout)
		case "Stderr":
			l.Push(r.Stderr)
		case "Combined":
			l.Push(r.Combined)
		case "Error":
			pushStr(l, r.Error)
		default:
			l.Push(nil)
		}
		return 1
	})
	l.SetTableRaw(-3)

	l.Push("__newindex")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*ExecResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "ResponseTimeMS":
			r.ResponseTimeMS = l.ToFloat(3)
		case "ExitCode":
			r.ExitCode = int(l.ToInt(3))
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}

func pushStr(l *lua.State, s string) {
	if s == "" {
		l.Push(nil)
	} else {
		l.Push(s)
	}
}

func readStrSlice(l *lua.State, tableIdx int) []string {
	idx := l.AbsIndex(tableIdx)
	var out []string
	l.ForEachRaw(idx, func() bool {
		v := l.GetRaw(-1)
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
		return true
	})
	return out
}

func readStringOpt(l *lua.State, tableIdx int, key string, def string) string {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return def
	}
	v := l.GetRaw(-1)
	l.Pop(1)
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func readIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return def
	}
	v := l.GetRaw(-1)
	l.Pop(1)
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

func readStringMapOpt(l *lua.State, tableIdx int, key string) map[string]string {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil || t != lua.TypTable {
		l.Pop(1)
		return nil
	}

	result := make(map[string]string)
	subIdx := l.AbsIndex(-1)

	l.ForEachRaw(subIdx, func() bool {
		k := l.GetRaw(-2)
		if ks, ok := k.(string); ok {
			v := l.GetRaw(-1)
			if vs, ok := v.(string); ok {
				result[ks] = vs
			}
		}
		return true
	})

	l.Pop(1)
	if len(result) == 0 {
		return nil
	}
	return result
}
