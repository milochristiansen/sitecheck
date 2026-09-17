package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/milochristiansen/lua"

	"sitecheck/cmd/scoutpost/lmods"
	"sitecheck/core"
)

// Resource represents a single check script discovered in the resources directory.
type Resource struct {
	Slug           string
	ScriptPath     string
	Name           string
	Desc           string
	Skip           bool
	NotifyPass     bool
	NotifyDegraded bool
	NotifyFail     bool
	Sites          map[string]string // site name → detail level
}

// toRegistryMeta converts this Resource to a core.ResourceMeta for plugin dispatch.
func (r Resource) toRegistryMeta() core.ResourceMeta {
	return core.ResourceMeta{
		Slug:           r.Slug,
		Name:           r.Name,
		Desc:           r.Desc,
		NotifyPass:     r.NotifyPass,
		NotifyDegraded: r.NotifyDegraded,
		NotifyFail:     r.NotifyFail,
	}
}

// Job is a single check script to execute.
type Job struct {
	Resource Resource
}

// Pool manages a fixed set of worker goroutines that execute check jobs concurrently, each with its own Lua state.
// Results are sent as core.WireResult values.
type Pool struct {
	jobs           chan Job
	results        chan core.WireResult
	defaultTimeout int
	wg             sync.WaitGroup
}

// NewPool creates n workers and returns a Pool ready to accept jobs.
func NewPool(n int, defaultTimeout int) *Pool {
	p := &Pool{
		jobs:           make(chan Job),
		results:        make(chan core.WireResult),
		defaultTimeout: defaultTimeout,
	}
	for i := range n {
		id := i
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			debugf("worker[%d]: starting", id)
			l, err := lmods.NewState(defaultTimeout)
			if err != nil {
				debugf("worker[%d]: create lua state failed: %v", id, err)
				p.results <- luaErrorResult(Resource{}, fmt.Sprintf("create lua state: %v", err), 0)
				return
			}
			debugf("worker[%d]: ready", id)
			for job := range p.jobs {
				debugf("worker[%d]: got job slug=%q", id, job.Resource.Slug)
				if job.Resource.Skip {
					debugf("worker[%d]: slug=%q skipped before execution", id, job.Resource.Slug)
					continue
				}
				// Load and execute the Lua script file.
				if err := lmods.ExecuteFile(l, job.Resource.ScriptPath); err != nil {
					debugf("worker[%d]: execute %q failed: %v", id, job.Resource.ScriptPath, err)
					p.results <- luaErrorResult(job.Resource, err.Error(), 0)
					continue
				}
				debugf("worker[%d]: loaded script slug=%q", id, job.Resource.Slug)
				// Run meta() to populate resource fields.
				if err := PopulateMeta(l, &job.Resource); err != nil {
					debugf("worker[%d]: meta() slug=%q failed: %v", id, job.Resource.Slug, err)
					p.results <- luaErrorResult(job.Resource, err.Error(), 0)
					continue
				}
				debugf("worker[%d]: meta slug=%q name=%q skipped=%t", id, job.Resource.Slug, job.Resource.Name, job.Resource.Skip)
				if job.Resource.Skip {
					debugf("worker[%d]: slug=%q skipped by meta", id, job.Resource.Slug)
					continue
				}
				wr := RunCheckWithRetry(l, job.Resource)
				debugf("worker[%d]: check slug=%q done pass=%d elapsed_ms=%d err=%q",
					id, wr.Slug, wr.Pass, wr.ElapsedMS, wr.Error)
				p.results <- wr
			}
			debugf("worker[%d]: exiting", id)
		}()
	}
	return p
}

// Submit enqueues a job for execution.
func (p *Pool) Submit(job Job) {
	p.jobs <- job
}

// Wait closes the job channel, waits for all workers to finish, then closes the results channel.
func (p *Pool) Wait() {
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
}

// Results returns the channel on which check results are delivered.
func (p *Pool) Results() <-chan core.WireResult {
	return p.results
}

// ScanResources finds all .lua files in dir and returns Resources with slugs derived from filenames.
func ScanResources(dir string) ([]Resource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read resources dir %s: %w", dir, err)
	}
	var resources []Resource
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".lua")
		resources = append(resources, Resource{
			Slug:       slug,
			ScriptPath: filepath.Join(dir, e.Name()),
			Name:       titleCase(slug),
		})
	}
	return resources, nil
}

// PopulateMeta executes the script's meta() function if present, filling in Name, Description, and notify fields on the
// resource.
func PopulateMeta(l *lua.State, res *Resource) error {
	ok, err := core.CallMeta(l)
	if err != nil {
		return fmt.Errorf("call meta() for %s: %w", res.Slug, err)
	}
	if !ok {
		return nil
	}

	if l.TypeOf(-1) == lua.TypTable {
		name := core.ReadStringField(l, -1, "name", "")
		desc := core.ReadStringField(l, -1, "description", "")
		if name != "" {
			res.Name = name
		}
		if desc != "" {
			res.Desc = desc
		}

		res.Skip = core.ReadBoolField(l, -1, "skip", false)

		// Read notify sub-table if present. Values are booleans (wire protocol).
		l.Push("notify")
		if l.GetTableRaw(-2) == lua.TypTable {
			res.NotifyPass = core.ReadBoolField(l, -1, "pass", true)
			res.NotifyDegraded = core.ReadBoolField(l, -1, "degraded", true)
			res.NotifyFail = core.ReadBoolField(l, -1, "fail", true)
		}
		l.Pop(1)

		// Read sites sub-table if present. Map of site name → detail level.
		sites, err := core.ReadStringMap(l, -1, "sites")
		if err != nil {
			l.Pop(1)
			return fmt.Errorf("meta() for %s: %w", res.Slug, err)
		}
		res.Sites = sites
	}
	l.Pop(1)
	return nil
}

// runCheck executes a script's check() function exactly once and returns a
// core.WireResult. It is the single-attempt primitive; callers that want the
// outpost's normal retry-on-failure behaviour should use RunCheck instead.
func runCheck(l *lua.State, res Resource) core.WireResult {
	l.Push("check")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return luaErrorResult(res, fmt.Sprintf("resource %s: check() function not found", res.Slug), 0)
	}

	debugf("check %s: invoking check()", res.Slug)
	start := time.Now()
	err := l.Protect(func() {
		l.Call(0, 1)
	})
	elapsed := time.Since(start)
	debugf("check %s: check() returned after %s (err=%v)", res.Slug, elapsed.Round(time.Millisecond), err)

	if err != nil {
		l.Pop(1)
		return luaErrorResult(res, err.Error(), elapsed.Milliseconds())
	}

	if l.TypeOf(-1) != lua.TypUserData {
		l.Pop(1)
		return luaErrorResult(res, "check() did not return userdata", elapsed.Milliseconds())
	}

	raw := l.ToUser(-1)
	l.Pop(1)

	cr, ok := raw.(core.CheckResult)
	if !ok {
		return luaErrorResult(res, fmt.Sprintf("check() result type %T does not implement CheckResult", raw), elapsed.Milliseconds())
	}

	debugf("check %s: dispatching type=%q", res.Slug, cr.CheckType())
	p, ok := core.ByName(cr.CheckType())
	if !ok {
		return luaErrorResult(res, fmt.Sprintf("unknown check type %q", cr.CheckType()), elapsed.Milliseconds())
	}
	wr := p.DispatchWireResult(res.toRegistryMeta(), cr, elapsed)
	wr.Sites = res.Sites
	wr.Version = core.WireVersion
	return wr
}

// checkPassed reports whether wr is a result the rest of SiteCheck will treat
// as passing. A check that is not at the PASS level, or that carries an error,
// is considered a failure worth retrying.
func checkPassed(wr core.WireResult) bool {
	return wr.Pass == core.PASS && wr.Error == ""
}

// RunCheck executes a script's check() function and returns a core.WireResult.
//
// The outpost does not trust a single failure: when the first attempt does not
// pass, the check is immediately run a second time and the WireResult from that
// second attempt is returned, pass or fail. This keeps a transient network blip
// from being reported as a real outage for a whole check cycle, while a failure
// that the second attempt reproduces is still reported normally.
func RunCheck(l *lua.State, res Resource) core.WireResult {
	return RunCheckWithRetry(l, res)
}

// RunCheckWithRetry is RunCheck under an explicit name. It attempts the check
// once and, if that attempt does not pass, attempts it a second time. The
// result of the second attempt is returned regardless of its outcome.
func RunCheckWithRetry(l *lua.State, res Resource) core.WireResult {
	wr := runCheck(l, res)
	if checkPassed(wr) {
		return wr
	}
	debugf("check %s: attempt failed (pass=%d error=%q); retrying once", res.Slug, wr.Pass, wr.Error)
	retry := runCheck(l, res)
	debugf("check %s: retry returned pass=%d elapsed_ms=%d error=%q", res.Slug, retry.Pass, retry.ElapsedMS, retry.Error)
	return retry
}

// luaErrorResult builds the Lua-error WireResult for a resource, carrying the
// resource's metadata and the given error message.
func luaErrorResult(res Resource, errMsg string, elapsedMS int64) core.WireResult {
	return core.WireResult{
		Slug:           res.Slug,
		Name:           res.Name,
		Desc:           res.Desc,
		NotifyPass:     res.NotifyPass,
		NotifyDegraded: res.NotifyDegraded,
		NotifyFail:     res.NotifyFail,
		Sites:          res.Sites,
		Version:        core.WireVersion,
		CheckType:      core.CheckTypeLuaError,
		Error:          errMsg,
		ElapsedMS:      elapsedMS,
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}
