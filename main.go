package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/joho/godotenv"

	"sitecheck/db"
	"sitecheck/lmods"
)

func main() {
	// Load .env from working directory; not fatal if missing — defaults apply.
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: no .env file found, using defaults (%v)\n", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// Open DB, run migrations.
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DB error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Migration error: %v\n", err)
		os.Exit(1)
	}

	// Scan resources directory for .lua scripts.
	resources, err := ScanResources(cfg.ResourcesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan error: %v\n", err)
		os.Exit(1)
	}

	if len(resources) == 0 {
		fmt.Printf("No .lua scripts found in %s — nothing to check.\n", cfg.ResourcesDir)
		return
	}

	// Create worker pool.
	pool := NewPool(cfg.Workers, cfg.DefaultTimeout)

	fmt.Printf("Running %d check(s) with %d worker(s)...\n", len(resources), cfg.Workers)
	for _, res := range resources {
		pool.Submit(Job{
			ScriptPath: res.ScriptPath,
			Slug:       res.Slug,
		})
	}
	pool.Wait()

	// Single-threaded collector drains results, writes to DB — no contention.
	var results []Result
	for result := range pool.Results() {
		if result.Raw == nil {
			fmt.Fprintf(os.Stderr, "  %-20s SKIP: %v\n", result.Slug, result.Err)
		} else if err := InsertTypedCheck(database, result); err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s DB ERROR: %v\n", result.Slug, err)
		} else {
			fmt.Printf("  %-20s type=%-6s %s %s\n", result.Slug, checkType(result), checkPassName(result), result.Elapsed.Round(1_000_000))
		}
		results = append(results, result)
	}

	// Query history for the largest graph window and populate each Result.
	maxWindow := 24
	for _, w := range cfg.GraphWindows {
		if w > maxWindow {
			maxWindow = w
		}
	}
	since := time.Now().Add(-time.Duration(maxWindow) * time.Hour)

	for i := range results {
		r := &results[i]
		if r.Raw == nil {
			continue
		}
		r.History = queryTypedHistory(database, r.Slug, checkType(*r), since)
	}

	// Sort alphabetically by slug.
	sort.Slice(results, func(i, j int) bool { return results[i].Slug < results[j].Slug })

	// Purge old checks.
	if err := database.PurgeOld(cfg.RetentionDays); err != nil {
		fmt.Fprintf(os.Stderr, "Purge error: %v\n", err)
	}

	// Generate static site.
	if err := Generate(cfg, results); err != nil {
		fmt.Fprintf(os.Stderr, "Sitegen error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done.")
}


func checkType(r Result) string {
	if cr, ok := r.Raw.(lmods.CheckResult); ok {
		return cr.CheckType()
	}
	return "???"
}

func checkPassName(r Result) string {
	if cr, ok := r.Raw.(lmods.CheckResult); ok {
		return passName(cr.CheckPass())
	}
	return "???"
}

func passName(p int) string {
	switch p {
	case 2:
		return "PASS"
	case 1:
		return "DEGRADED"
	default:
		return "FAIL"
	}
}

// queryTypedHistory returns the full typed DB check history for a slug+type.
func queryTypedHistory(database *db.DB, slug, checkType string, since time.Time) interface{} {
	switch checkType {
	case "http":
		h, err := db.HTTPChecksBySlugSince(database, slug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.HTTPCheck(nil)
		}
		return h
	case "ping":
		h, err := db.PingChecksBySlugSince(database, slug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.PingCheck(nil)
		}
		return h
	case "tcp":
		h, err := db.TCPChecksBySlugSince(database, slug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.TCPCheck(nil)
		}
		return h
	case "dns":
		h, err := db.DNSChecksBySlugSince(database, slug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.DNSCheck(nil)
		}
		return h
	case "ssl":
		h, err := db.SSLChecksBySlugSince(database, slug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.SSLCheck(nil)
		}
		return h
	default:
		return nil
	}
}
