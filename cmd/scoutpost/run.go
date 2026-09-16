package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"sitecheck/core"
)

// bearerToken extracts the token from an "Authorization: Bearer <token>" header value.
func bearerToken(auth string) string {
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// runChecks submits every resource to a fresh worker pool and streams the
// results as JSON-lines to out, flushing after each result when flush is
// non-nil. A write error stops the stream — the transport is unusable and
// later writes would fail identically.
func runChecks(cfg *Config, resources []Resource, out io.Writer, flush func()) {
	if len(resources) == 0 {
		debugf("run: no checks to run")
		return
	}
	pool := NewPool(cfg.Workers, cfg.DefaultTimeout)
	fmt.Fprintf(os.Stderr, "Running %d check(s) with %d worker(s)...\n", len(resources), cfg.Workers)
	debugf("run: created pool workers=%d jobs=%d", cfg.Workers, len(resources))

	go func() {
		for i, res := range resources {
			debugf("run: submit[%d/%d] slug=%q", i+1, len(resources), res.Slug)
			pool.Submit(Job{Resource: res})
			debugf("run: submitted[%d/%d] slug=%q", i+1, len(resources), res.Slug)
		}
		debugf("run: all jobs submitted; waiting for workers")
		pool.Wait()
		debugf("run: all workers finished; closing result stream")
	}()

	n := 0
	for wr := range pool.Results() {
		n++
		// Log before the write: if the process is blocked because the reader
		// stopped draining our stdout pipe, this is the last line emitted.
		debugf("run: writing result[%d] slug=%q pass=%d data_bytes=%d",
			n, wr.Slug, wr.Pass, len(wr.Data))
		if err := core.WriteResult(out, wr); err != nil {
			debugf("run: write result[%d] slug=%q failed: %v", n, wr.Slug, err)
			fmt.Fprintf(os.Stderr, "write result error: %v\n", err)
			return
		}
		debugf("run: wrote result[%d] slug=%q", n, wr.Slug)
		if flush != nil {
			flush()
		}
	}
	debugf("run: result stream drained after %d result(s)", n)
}
