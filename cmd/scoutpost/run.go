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
		return
	}
	pool := NewPool(cfg.Workers, cfg.DefaultTimeout)
	fmt.Fprintf(os.Stderr, "Running %d check(s) with %d worker(s)...\n", len(resources), cfg.Workers)
	go func() {
		for _, res := range resources {
			pool.Submit(Job{Resource: res})
		}
		pool.Wait()
	}()
	for wr := range pool.Results() {
		if err := core.WriteResult(out, wr); err != nil {
			fmt.Fprintf(os.Stderr, "write result error: %v\n", err)
			return
		}
		if flush != nil {
			flush()
		}
	}
}
