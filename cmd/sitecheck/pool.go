package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"sitecheck/checktypes/outpost"
	"sitecheck/core"
)

// PoolResult carries a single result from an outpost, or an error indicating the outpost as a whole failed.
type PoolResult struct {
	OutpostSlug string
	OutpostName string
	WireResult  *core.WireResult // nil when outpost failed
	Err         error            // non-nil when the outpost client failed
}

// runOutpostPool fans out across all active outposts concurrently, bounded by cfg.OutpostWorkers. Results are streamed
// on the returned channel as they arrive. The channel is closed when all outposts have finished.
func runOutpostPool(outposts []OutpostDef, cfg *Config) <-chan PoolResult {
	// Filter out skipped outposts.
	var active []OutpostDef
	for _, o := range outposts {
		if !o.Skip {
			active = append(active, o)
		}
	}

	ch := make(chan PoolResult)
	sem := make(chan struct{}, cfg.OutpostWorkers)

	var wg sync.WaitGroup
	for _, o := range active {
		wg.Add(1)
		go func(o OutpostDef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var client Client
			if o.Slug == "local" {
				client = NewLocalClient(cfg.OutpostBin, cfg.ResourcesDir, cfg.DefaultTimeout)
			} else {
				client = NewHTTPClient(o.URL, o.Token, cfg.DefaultTimeout)
			}

			start := time.Now()
			resultCh, err := client.Run()
			if err != nil {
				ch <- PoolResult{OutpostSlug: o.Slug, OutpostName: o.Name, Err: err}
				return
			}

			fmt.Printf("Running checks via outpost %q...\n", o.Slug)
			var totalChecks, failCount int
			var firstResult time.Time
			for wr := range resultCh {
				if totalChecks == 0 {
					firstResult = time.Now()
				}
				wr.OutpostSlug = o.Slug
				wrCopy := wr
				ch <- PoolResult{OutpostSlug: o.Slug, OutpostName: o.Name, WireResult: &wrCopy}
				totalChecks++
				if wr.Pass != core.PASS {
					failCount++
				}
			}

			// Emit a synthetic outpost health result.
			elapsed := time.Since(start)
			respMS := elapsed.Seconds() * 1000
			if !firstResult.IsZero() {
				respMS = firstResult.Sub(start).Seconds() * 1000
			}
			outpostData, _ := json.Marshal(outpost.OutpostResult{
				Pass:           core.PASS,
				ResponseTimeMS: respMS,
				CheckCount:     totalChecks,
				FailCount:      failCount,
			})
			ch <- PoolResult{
				OutpostSlug: o.Slug, OutpostName: o.Name,
				WireResult: &core.WireResult{
					Slug:        o.Slug,
					Name:        o.Name,
					CheckType:   "outpost",
					Pass:        core.PASS,
					ResponseMS:  respMS,
					ElapsedMS:   elapsed.Milliseconds(),
					Data:        outpostData,
					OutpostSlug: o.Slug,
					Sites:       o.Sites,
					Version:     core.WireVersion,
				},
			}
		}(o)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	return ch
}
