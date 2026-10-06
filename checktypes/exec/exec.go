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
	Stdout         string  `json:"stdout" hash:"StdoutHash"`
	StdoutHash     *string `json:"stdout_hash,omitempty"`
	Stderr         string  `json:"stderr" hash:"StderrHash"`
	StderrHash     *string `json:"stderr_hash,omitempty"`
	Combined       string  `json:"combined" hash:"CombinedHash"`
	CombinedHash   *string `json:"combined_hash,omitempty"`
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
	Stdout         string  `json:"stdout" hash:"StdoutHash"`
	StdoutHash     *string `json:"stdout_hash,omitempty"`
	Stderr         string  `json:"stderr" hash:"StderrHash"`
	StderrHash     *string `json:"stderr_hash,omitempty"`
	Combined       string  `json:"combined" hash:"CombinedHash"`
	CombinedHash   *string `json:"combined_hash,omitempty"`
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
			stdout_hash       TEXT,
			stderr            TEXT,
			stderr_hash       TEXT,
			combined          TEXT,
			combined_hash     TEXT,
			error             TEXT
		)`,
		`ALTER TABLE checks_exec ADD COLUMN stdout_hash TEXT`,
		`ALTER TABLE checks_exec ADD COLUMN stderr_hash TEXT`,
		`ALTER TABLE checks_exec ADD COLUMN combined_hash TEXT`,
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

	stdout := truncate(r.Stdout)
	stderr := truncate(r.Stderr)
	combined := truncate(r.Combined)
	_, err := db.Exec(
		`INSERT INTO checks_exec
			(slug, outpost_slug, duration_ms, pass, response_time_ms,
			 command, exit_code, stdout, stdout_hash, stderr, stderr_hash, combined, combined_hash, error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.Command, r.ExitCode, stdout, core.ContentHash(stdout), stderr, core.ContentHash(stderr), combined, core.ContentHash(combined), r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert exec check: %w", err)
	}
	return nil
}

func (p *plugin) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_exec
			(slug, outpost_slug, duration_ms, pass, error, stdout_hash, stderr_hash, combined_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, errMsg,
		core.ContentHash(""), core.ContentHash(""), core.ContentHash(""),
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
			command, exit_code, stdout, stdout_hash, stderr, stderr_hash, combined, combined_hash, error
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
			c            ExecCheck
			durationMS   sql.NullInt64
			responseMS   sql.NullFloat64
			command      sql.NullString
			exitCode     sql.NullInt64
			stdout       sql.NullString
			stdoutHash   sql.NullString
			stderr       sql.NullString
			stderrHash   sql.NullString
			combined     sql.NullString
			combinedHash sql.NullString
			errMsg       sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &command, &exitCode, &stdout, &stdoutHash, &stderr, &stderrHash, &combined, &combinedHash, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan exec check: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Command = command.String
		c.ExitCode = int(exitCode.Int64)
		c.Stdout = stdout.String
		if stdoutHash.Valid {
			h := stdoutHash.String
			c.StdoutHash = &h
		}
		c.Stderr = stderr.String
		if stderrHash.Valid {
			h := stderrHash.String
			c.StderrHash = &h
		}
		c.Combined = combined.String
		if combinedHash.Valid {
			h := combinedHash.String
			c.CombinedHash = &h
		}
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

// --- Bounded history reads --------------------------------------------------

// QueryPoints returns narrow numeric history for sparklines, charts, and stats.
func (p *plugin) QueryPoints(db *sql.DB, slug, outpostSlug string, since time.Time, limit int) ([]core.CheckPoint, error) {
	return core.QueryPoints(db, p.TableName(), slug, outpostSlug, since, limit)
}

// EachRecentLight streams light rows (stdout/stderr/combined omitted, hashes
// included) newest-first.
func (p *plugin) EachRecentLight(db *sql.DB, slug, outpostSlug string, since time.Time, fn func(id string, row interface{}) error) error {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			command, exit_code, stdout_hash, stderr_hash, combined_hash, error
		FROM checks_exec WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp DESC`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return fmt.Errorf("query exec light rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			c            ExecCheck
			durationMS   sql.NullInt64
			responseMS   sql.NullFloat64
			command      sql.NullString
			exitCode     sql.NullInt64
			stdoutHash   sql.NullString
			stderrHash   sql.NullString
			combinedHash sql.NullString
			errMsg       sql.NullString
		)
		if err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &command, &exitCode, &stdoutHash, &stderrHash, &combinedHash, &errMsg); err != nil {
			return fmt.Errorf("scan exec light row: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.Command = command.String
		c.ExitCode = int(exitCode.Int64)
		if stdoutHash.Valid {
			h := stdoutHash.String
			c.StdoutHash = &h
		}
		if stderrHash.Valid {
			h := stderrHash.String
			c.StderrHash = &h
		}
		if combinedHash.Valid {
			h := combinedHash.String
			c.CombinedHash = &h
		}
		c.Error = errMsg.String
		if err := fn(c.ID, c); err != nil {
			return err
		}
	}
	return rows.Err()
}

// LoadFull returns the fully hydrated exec row for a primary key.
func (p *plugin) LoadFull(db *sql.DB, id string) (interface{}, error) {
	var (
		c            ExecCheck
		durationMS   sql.NullInt64
		responseMS   sql.NullFloat64
		command      sql.NullString
		exitCode     sql.NullInt64
		stdout       sql.NullString
		stdoutHash   sql.NullString
		stderr       sql.NullString
		stderrHash   sql.NullString
		combined     sql.NullString
		combinedHash sql.NullString
		errMsg       sql.NullString
	)
	err := db.QueryRow(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			command, exit_code, stdout, stdout_hash, stderr, stderr_hash, combined, combined_hash, error
		FROM checks_exec WHERE id = ?`, id,
	).Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass, &responseMS,
		&command, &exitCode, &stdout, &stdoutHash, &stderr, &stderrHash, &combined, &combinedHash, &errMsg)
	if err != nil {
		return nil, fmt.Errorf("load exec check %s: %w", id, err)
	}
	c.DurationMS = durationMS.Int64
	c.ResponseTimeMS = responseMS.Float64
	c.Command = command.String
	c.ExitCode = int(exitCode.Int64)
	c.Stdout = stdout.String
	if stdoutHash.Valid {
		h := stdoutHash.String
		c.StdoutHash = &h
	}
	c.Stderr = stderr.String
	if stderrHash.Valid {
		h := stderrHash.String
		c.StderrHash = &h
	}
	c.Combined = combined.String
	if combinedHash.Valid {
		h := combinedHash.String
		c.CombinedHash = &h
	}
	c.Error = errMsg.String
	return c, nil
}

// BackfillHashes fills the output hashes for rows inserted before hashing
// existed. It is batched and idempotent. NULL output is hashed as the empty
// string, matching new inserts.
func (p *plugin) BackfillHashes(db *sql.DB) error {
	const batch = 512
	for {
		rows, err := db.Query(
			`SELECT id, COALESCE(stdout, ''), COALESCE(stderr, ''), COALESCE(combined, '')
			 FROM checks_exec
			 WHERE stdout_hash IS NULL OR stderr_hash IS NULL OR combined_hash IS NULL
			 LIMIT ?`, batch)
		if err != nil {
			return fmt.Errorf("backfill exec hash query: %w", err)
		}
		type item struct {
			id       string
			stdout   string
			stderr   string
			combined string
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.id, &it.stdout, &it.stderr, &it.combined); err != nil {
				rows.Close()
				return fmt.Errorf("backfill exec hash scan: %w", err)
			}
			items = append(items, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("backfill exec hash rows: %w", err)
		}
		if len(items) == 0 {
			return nil
		}
		for _, it := range items {
			if _, err := db.Exec(
				`UPDATE checks_exec SET stdout_hash = ?, stderr_hash = ?, combined_hash = ? WHERE id = ?`,
				core.ContentHash(it.stdout), core.ContentHash(it.stderr), core.ContentHash(it.combined), it.id,
			); err != nil {
				return fmt.Errorf("backfill exec hash %s: %w", it.id, err)
			}
		}
	}
}

// NeedsHydration reports that light rows omit stdout/stderr/combined.
func (p *plugin) NeedsHydration() bool { return true }

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
