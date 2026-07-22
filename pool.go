package main

import (
	"fmt"
	"os"
	"sync"

	"sitecheck/lmods"
)

// Job is a single check script to execute.
type Job struct {
	ScriptPath string
	Slug       string
}

// Pool manages a fixed set of worker goroutines that execute check jobs
// concurrently, each with its own Lua state. Results are sent to a buffered
// channel for a single collector to drain.
type Pool struct {
	jobs           chan Job
	results        chan Result
	defaultTimeout int
	wg             sync.WaitGroup
}

// NewPool creates n workers and returns a Pool ready to accept jobs.
// Each job gets a fresh Lua state created and destroyed within the worker —
// no state is shared or reused across jobs.
func NewPool(n int, defaultTimeout int) *Pool {
	p := &Pool{
		jobs:           make(chan Job),
		results:        make(chan Result),
		defaultTimeout: defaultTimeout,
	}

	for range n {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for job := range p.jobs {
				l, err := lmods.NewState(p.defaultTimeout)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s ERROR state: %v\n", job.Slug, err)
					p.results <- Result{Slug: job.Slug, Err: err.Error()}
					continue
				}

				res := Resource{Slug: job.Slug, ScriptPath: job.ScriptPath}

				// Load the script once; both meta() and check() run on the same state.
				if err := lmods.ExecuteFile(l, res.ScriptPath); err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s ERROR load: %v\n", job.Slug, err)
					p.results <- Result{Slug: job.Slug, Err: err.Error()}
					continue
				}

				// Populate metadata from meta().
				if err := PopulateMeta(l, &res); err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s meta() error: %v\n", job.Slug, err)
				}

				result, err := RunCheck(l, res)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s ERROR: %v\n", job.Slug, err)
					if result.Slug == "" {
						result = Result{
							Slug: job.Slug,
							Name: res.Name,
							Desc: res.Desc,
							Err:  err.Error(),
						}
					}
				}
				p.results <- result
			}
		}()
	}

	return p
}

// Submit enqueues a job for execution. Blocks until a worker accepts the job.
func (p *Pool) Submit(job Job) {
	p.jobs <- job
}

// Wait closes the job channel, waits for all workers to finish, then closes
// the results channel. Call this after all Submit calls; no further jobs
// can be submitted.
func (p *Pool) Wait() {
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
}

// Results returns the channel on which check results are delivered.
// The channel is closed after Wait() completes.
func (p *Pool) Results() <-chan Result {
	return p.results
}

