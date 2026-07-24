package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/milochristiansen/lua"

	"sitecheck/protocol"
	"sitecheck/cmd/scoutpost/lmods"
)

// Resource represents a single check script discovered in the resources directory.
type Resource struct {
	Slug            string
	ScriptPath      string
	Name            string
	Desc            string
	Skip            bool
	NotifyPass      bool
	NotifyDegraded  bool
	NotifyFail      bool
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
			for job := range p.jobs {
				l, err := lmods.NewState(p.defaultTimeout)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s ERROR state: %v\n", job.Resource.Slug, err)
					p.results <- protocol.WireResult{
						Slug:  job.Resource.Slug,
						Error: err.Error(),
					}
					continue
				}

				res := job.Resource

				// Load the script once; both meta() and check() run on the same state.
				if err := lmods.ExecuteFile(l, res.ScriptPath); err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s ERROR load: %v\n", res.Slug, err)
					p.results <- protocol.WireResult{
						Slug:  res.Slug,
						Error: err.Error(),
					}
					continue
				}

				// Populate metadata from meta().
				if err := PopulateMeta(l, &res); err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s meta() error: %v\n", res.Slug, err)
				}

				// If the resource is marked as skipped, drop the job entirely.
				if res.Skip {
					fmt.Fprintf(os.Stderr, "  %-20s SKIP\n", res.Slug)
					continue
				}

				result := RunCheck(l, res)
				p.results <- result
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
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".lua")
		resources = append(resources, Resource{
			Slug:       slug,
			ScriptPath: filepath.Join(dir, entry.Name()),
			Name:       titleCase(slug),
			Desc:       "",
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
			res.NotifyPass = lmods.ReadBoolField(l, -1, "pass", false)
			res.NotifyDegraded = lmods.ReadBoolField(l, -1, "degraded", false)
			res.NotifyFail = lmods.ReadBoolField(l, -1, "fail", true)
		}
		l.Pop(1)
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
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
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
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
			Error:          "check() did not return userdata",
		}
	}

	raw := l.ToUser(-1)
	l.Pop(1)

	// Dispatch based on the typed result.
	switch r := raw.(type) {
	case *protocol.HTTPResult:
		return protocol.NewWireResult(
			res.Slug, res.Name, res.Desc,
			"http", r.Pass, r.FailReason,
			r.ResponseTimeMS, elapsed.Milliseconds(),
			r.Error,
			r,
			res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
		)
	case *protocol.PingResult:
		return protocol.NewWireResult(
			res.Slug, res.Name, res.Desc,
			"ping", r.Pass, r.FailReason,
			r.ResponseTimeMS, elapsed.Milliseconds(),
			r.Error,
			r,
			res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
		)
	case *protocol.TCPResult:
		return protocol.NewWireResult(
			res.Slug, res.Name, res.Desc,
			"tcp", r.Pass, r.FailReason,
			r.ResponseTimeMS, elapsed.Milliseconds(),
			r.Error,
			r,
			res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
		)
	case *protocol.DNSResult:
		return protocol.NewWireResult(
			res.Slug, res.Name, res.Desc,
			"dns", r.Pass, r.FailReason,
			r.ResponseTimeMS, elapsed.Milliseconds(),
			r.Error,
			r,
			res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
		)
	case *protocol.SSLResult:
		return protocol.NewWireResult(
			res.Slug, res.Name, res.Desc,
			"ssl", r.Pass, r.FailReason,
			r.ResponseTimeMS, elapsed.Milliseconds(),
			r.Error,
			r,
			res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
		)
	case *protocol.SystemdResult:
		return protocol.NewWireResult(
			res.Slug, res.Name, res.Desc,
			"systemd", r.Pass, r.FailReason,
			r.ResponseTimeMS, elapsed.Milliseconds(),
			r.Error,
			r,
			res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
		)
	default:
		return protocol.WireResult{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			NotifyFail:     res.NotifyFail,
			ElapsedMS:      elapsed.Milliseconds(),
			Error:          fmt.Sprintf("unknown result type %T", raw),
		}
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
