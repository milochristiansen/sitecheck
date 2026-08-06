// Package exec implements core.CheckPlugin for arbitrary command-execution checks.
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

	"sitecheck/core"
)

// maxOutputBytes is the per-field truncation limit for stdout, stderr, and
// combined when stored in the database or serialized on the wire.
const maxOutputBytes = 64 << 10 // 64 KiB

// --- Result struct ----------------------------------------------------------

// ExecResult is the check-type-specific result for a command execution.
// It implements core.CheckResult.
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
func (r *ExecResult) CheckPass() int           { return r.Pass }
func (r *ExecResult) CheckFailReason() string  { return r.FailReason }
func (r *ExecResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

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

func (p *plugin) ExtractPoints(history interface{}) []core.CheckPoint {
	h, ok := history.([]ExecCheck)
	if !ok {
		return nil
	}
	pts := make([]core.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = core.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (p *plugin) ExtractDurationPoints(_ interface{}) []core.CheckPoint {
	return nil
}

func (p *plugin) LatestRecent(history interface{}) (latest, recent interface{}, count int) {
	h, ok := history.([]ExecCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	latest = &h[len(h)-1]
	if len(h) == 1 {
		return latest, nil, 0
	}
	n := len(h) - 1
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
			args = core.ReadStrSlice(l, 2)
		}

		// Options table (third argument).
		timeout := defaultTimeout
		var env map[string]string
		var stdin string
		if !l.IsNil(3) && l.TypeOf(3) == lua.TypTable {
			timeout = core.ReadIntOpt(l, 3, "timeout", defaultTimeout)
			env = core.ReadStringMapOpt(l, 3, "env")
			stdin = core.ReadStringOpt(l, 3, "stdin", "")
		}

		r := &ExecResult{
			Pass:    core.FAIL,
			Command: formatCommand(name, args),
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, name, args...)

		// Set environment if provided.
		if len(env) > 0 {
			cmd.Env = os.Environ()
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
		r.ResponseTimeMS = elapsed.Seconds() * 1000

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

func (p *plugin) DispatchWireResult(res core.ResourceMeta, cr core.CheckResult, elapsed time.Duration) core.WireResult {
	r := cr.(*ExecResult)
	return core.NewWireResult(
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

// --- Registration ------------------------------------------------------------

func init() {
	core.Register(&plugin{})
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

func pushExecResult(l *lua.State, r *ExecResult) {
	core.PushResultUserData(l, r, map[string]core.LuaField{
		"Pass":           {Get: func(l *lua.State) { l.Push(int64(r.Pass)) }, Set: func(l *lua.State) { r.Pass = int(l.ToInt(3)) }},
		"FailReason":     {Get: func(l *lua.State) { l.Push(r.FailReason) }, Set: func(l *lua.State) { r.FailReason = l.ToString(3) }},
		"ResponseTimeMS": {Get: func(l *lua.State) { l.Push(r.ResponseTimeMS) }},
		"Command":        {Get: func(l *lua.State) { l.Push(r.Command) }, Set: func(l *lua.State) { r.Command = l.ToString(3) }},
		"ExitCode":       {Get: func(l *lua.State) { l.Push(int64(r.ExitCode)) }},
		"Stdout":         {Get: func(l *lua.State) { l.Push(r.Stdout) }, Set: func(l *lua.State) { r.Stdout = l.ToString(3) }},
		"Stderr":         {Get: func(l *lua.State) { l.Push(r.Stderr) }, Set: func(l *lua.State) { r.Stderr = l.ToString(3) }},
		"Combined":       {Get: func(l *lua.State) { l.Push(r.Combined) }, Set: func(l *lua.State) { r.Combined = l.ToString(3) }},
		"Error":          {Get: func(l *lua.State) { l.Push(r.Error) }, Set: func(l *lua.State) { r.Error = l.ToString(3) }},
	})
}
