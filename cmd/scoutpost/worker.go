package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/milochristiansen/lua"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
	"sitecheck/cmd/scoutpost/lmods"
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

// toRegistryMeta converts this Resource to a registry.ResourceMeta for plugin dispatch.
func (r Resource) toRegistryMeta() registry.ResourceMeta {
	return registry.ResourceMeta{
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
// Results are sent as protocol.WireResult values.
type Pool struct {
	jobs           chan Job
	results        chan protocol.WireResult
	defaultTimeout int
	wg             sync.WaitGroup
}

// NewPool creates n workers and returns a Pool ready to accept jobs.
func NewPool(n int, defaultTimeout int) *Pool {
	p := &Pool{
		jobs:           make(chan Job),
		results:        make(chan protocol.WireResult),
		defaultTimeout: defaultTimeout,
	}
	for range n {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			l, err := lmods.NewState(defaultTimeout)
			if err != nil {
				p.results <- protocol.WireResult{CheckType: protocol.CheckTypeLuaError, Error: fmt.Sprintf("create lua state: %v", err), Version: protocol.WireVersion}
				return
			}
			for job := range p.jobs {
				if job.Resource.Skip {
					continue
				}
				// Load and execute the Lua script file.
				if err := lmods.ExecuteFile(l, job.Resource.ScriptPath); err != nil {
					p.results <- protocol.WireResult{
						Slug:           job.Resource.Slug,
						Name:           job.Resource.Name,
						Desc:           job.Resource.Desc,
						NotifyPass:     job.Resource.NotifyPass,
						NotifyDegraded: job.Resource.NotifyDegraded,
						NotifyFail:     job.Resource.NotifyFail,
						Sites:          job.Resource.Sites,
						Version:        protocol.WireVersion,
						CheckType:      protocol.CheckTypeLuaError,
						Error:          err.Error(),
					}
					continue
				}
				// Run meta() to populate resource fields.
				if err := PopulateMeta(l, &job.Resource); err != nil {
					p.results <- protocol.WireResult{
						Slug:           job.Resource.Slug,
						Name:           job.Resource.Name,
						Desc:           job.Resource.Desc,
						NotifyPass:     job.Resource.NotifyPass,
						NotifyDegraded: job.Resource.NotifyDegraded,
						NotifyFail:     job.Resource.NotifyFail,
						Sites:          job.Resource.Sites,
						Version:        protocol.WireVersion,
						CheckType:      protocol.CheckTypeLuaError,
						Error:          err.Error(),
					}
					continue
				}
				if job.Resource.Skip {
					continue
				}
				p.results <- RunCheck(l, job.Resource)
			}
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
func (p *Pool) Results() <-chan protocol.WireResult {
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
	l.Push("meta")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return nil
	}

	err := l.Protect(func() {
		l.Call(0, 1)
	})
	if err != nil {
		l.Pop(1)
		return fmt.Errorf("call meta() for %s: %w", res.Slug, err)
	}

	if l.TypeOf(-1) == lua.TypTable {
		name := lmods.ReadStringField(l, -1, "name", "")
		desc := lmods.ReadStringField(l, -1, "description", "")
		if name != "" {
			res.Name = name
		}
		if desc != "" {
			res.Desc = desc
		}

		res.Skip = lmods.ReadBoolField(l, -1, "skip", false)

		// Read notify sub-table if present. Values are booleans (wire protocol).
		l.Push("notify")
		if l.GetTableRaw(-2) == lua.TypTable {
			res.NotifyPass = lmods.ReadBoolField(l, -1, "pass", true)
			res.NotifyDegraded = lmods.ReadBoolField(l, -1, "degraded", true)
			res.NotifyFail = lmods.ReadBoolField(l, -1, "fail", true)
		}
		l.Pop(1)

		// Read sites sub-table if present. Map of site name → detail level.
		sites, err := lmods.ReadStringMap(l, -1, "sites")
		if err != nil {
			l.Pop(1)
			return fmt.Errorf("meta() for %s: %w", res.Slug, err)
		}
		res.Sites = sites
	}
	l.Pop(1)
	return nil
}

// RunCheck executes a script's check() function and returns a protocol.WireResult.
func RunCheck(l *lua.State, res Resource) protocol.WireResult {
	l.Push("check")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return protocol.WireResult{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			NotifyFail:     res.NotifyFail,
			Sites:          res.Sites,
			Version:        protocol.WireVersion,
			CheckType:      protocol.CheckTypeLuaError,
			Error:          fmt.Sprintf("resource %s: check() function not found", res.Slug),
		}
	}

	start := time.Now()
	err := l.Protect(func() {
		l.Call(0, 1)
	})
	elapsed := time.Since(start)

	if err != nil {
		l.Pop(1)
		return protocol.WireResult{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			Sites:          res.Sites,
			Version:        protocol.WireVersion,
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
			CheckType:      protocol.CheckTypeLuaError,
			Error:          err.Error(),
		}
	}

	if l.TypeOf(-1) != lua.TypUserData {
		l.Pop(1)
		return protocol.WireResult{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			Sites:          res.Sites,
			Version:        protocol.WireVersion,
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
			CheckType:      protocol.CheckTypeLuaError,
			Error:          "check() did not return userdata",
		}
	}

	raw := l.ToUser(-1)
	l.Pop(1)

	cr, ok := raw.(protocol.CheckResult)
	if !ok {
		return protocol.WireResult{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			Sites:          res.Sites,
			Version:        protocol.WireVersion,
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
			CheckType:      protocol.CheckTypeLuaError,
			Error:          fmt.Sprintf("check() result type %T does not implement CheckResult", raw),
		}
	}

	p, ok := registry.ByName(cr.CheckType())
	if !ok {
		return protocol.WireResult{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			Sites:          res.Sites,
			Version:        protocol.WireVersion,
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
			CheckType:      protocol.CheckTypeLuaError,
			Error:          fmt.Sprintf("unknown check type %q", cr.CheckType()),
		}
	}
	wr := p.DispatchWireResult(res.toRegistryMeta(), cr, elapsed)
	wr.Sites = res.Sites
	wr.Version = protocol.WireVersion
	return wr
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
