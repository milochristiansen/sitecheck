package main

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"time"

	"sitecheck/db"
	"sitecheck/lmods"
)

// IndexData is the template data for index.html.
type IndexData struct {
	Title         string
	SiteTitle     string
	Generated     string
	StaticPrefix  string
	Entries       []ResourceCard
	UpCount       int
	DegradedCount int
	DownCount     int
}

// ResourceCard holds per-resource display data for the overview page.
type ResourceCard struct {
	Slug       string
	Name       string
	CheckType  string
	Pass       int // 2=pass, 1=degraded, 0=fail, -1=no data
	ResponseMS float64
	Uptime24h  float64
	Sparkline  template.HTML
	FailReason string
}

// ResourcePage holds all data for a resource detail page.
type ResourcePage struct {
	Title        string
	SiteTitle    string
	Generated    string
	StaticPrefix string
	Slug         string
	Name         string
	Description  string
	CheckType    string
	Pass         int
	FailReason   string
	ResponseMS   float64

	// Stats
	Uptime24h     float64
	Uptime7d      float64
	Uptime30d     float64
	AvgResponseMS float64
	MinResponseMS float64
	MaxResponseMS float64
	TotalChecks   int

	// Charts: one SVG per graph window (keyed by window hours)
	Charts       map[int]template.HTML
	GraphWindows []int

	// Latest check row (typed DB struct pointer e.g. *db.HTTPCheck)
	LatestCheck interface{}

	// Recent checks for the collapsible table (typed DB slice, newest first)
	RecentChecks interface{}
	RecentCount  int
}

const maxRecentChecks = 15

// Generate renders the static site into cfg.OutputDir from the sorted
// slice of Results (each with History populated by the collector).
func Generate(cfg *Config, results []Result) error {
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	if err := copyDir(cfg.StaticDir, filepath.Join(cfg.OutputDir, "static")); err != nil {
		return fmt.Errorf("copy static: %w", err)
	}

	// Parse separate template sets — each page type has its own "content" block.
	indexTmpl, err := template.New("").Funcs(tmplFuncs()).ParseFiles(
		filepath.Join(cfg.TemplatesDir, "base.html"),
		filepath.Join(cfg.TemplatesDir, "index.html"),
	)
	if err != nil {
		return fmt.Errorf("parse index templates: %w", err)
	}

	detailTmpl, err := template.New("").Funcs(tmplFuncs()).ParseFiles(
		filepath.Join(cfg.TemplatesDir, "base.html"),
		filepath.Join(cfg.TemplatesDir, "resource.html"),
	)
	if err != nil {
		return fmt.Errorf("parse detail templates: %w", err)
	}

	// --- Overview page ---
	cards := buildCards(results)
	up, deg, down := countStatuses(cards)

	data := IndexData{
		Title:         cfg.SiteTitle + " — Overview",
		SiteTitle:     cfg.SiteTitle,
		Generated:     time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		StaticPrefix:  "static/",
		Entries:       cards,
		UpCount:       up,
		DegradedCount: deg,
		DownCount:     down,
	}

	outPath := filepath.Join(cfg.OutputDir, "index.html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if err := indexTmpl.ExecuteTemplate(f, "base.html", data); err != nil {
		return fmt.Errorf("render index: %w", err)
	}
	fmt.Printf("  Wrote %s\n", outPath)

	// --- Detail pages ---
	resourcesDir := filepath.Join(cfg.OutputDir, "resources")
	for _, r := range results {
		page := buildResourcePage(cfg, r)
		if err := writeDetailPage(detailTmpl, resourcesDir, page); err != nil {
			return err
		}
	}

	return nil
}

func buildCards(results []Result) []ResourceCard {
	cards := make([]ResourceCard, 0, len(results))

	for _, r := range results {
		card := ResourceCard{
			Slug: r.Slug,
			Name: r.Name,
			Pass: -1,
		}

		if cr, ok := r.Raw.(lmods.CheckResult); ok {
			card.CheckType = cr.CheckType()
			card.Pass = cr.CheckPass()
			card.FailReason = cr.CheckFailReason()
			card.ResponseMS = cr.CheckResponseMS()
		}

		pts := extractPoints(r.History)
		if len(pts) > 0 {
			card.Uptime24h = calcUptimePct(lastNHours(pts, 24))
			card.Sparkline = Sparkline(pts, 120, 30)
		}

		cards = append(cards, card)
	}
	return cards
}

func countStatuses(cards []ResourceCard) (up, degraded, down int) {
	for _, c := range cards {
		switch c.Pass {
		case 2:
			up++
		case 1:
			degraded++
		case 0:
			down++
		}
	}
	return
}

// buildResourcePage constructs a ResourcePage from a single Result.
func buildResourcePage(cfg *Config, r Result) ResourcePage {
	page := ResourcePage{
		Title:        r.Name + " — " + cfg.SiteTitle,
		SiteTitle:    cfg.SiteTitle,
		Generated:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		StaticPrefix: "../../static/",
		Slug:         r.Slug,
		Name:         r.Name,
		Description:  r.Desc,
		GraphWindows: cfg.GraphWindows,
	}

	if cr, ok := r.Raw.(lmods.CheckResult); ok {
		page.CheckType = cr.CheckType()
		page.Pass = cr.CheckPass()
		page.FailReason = cr.CheckFailReason()
		page.ResponseMS = cr.CheckResponseMS()
	}

	pts := extractPoints(r.History)
	if len(pts) > 0 {
		page.TotalChecks = len(pts)
		page.AvgResponseMS, page.MinResponseMS, page.MaxResponseMS = calcRespStats(pts)
		page.Uptime24h = calcUptimePct(lastNHours(pts, 24))
		page.Uptime7d = calcUptimePct(lastNHours(pts, 7*24))
		page.Uptime30d = calcUptimePct(pts)
	}

	// Charts per window.
	page.Charts = make(map[int]template.HTML)
	for _, w := range cfg.GraphWindows {
		windowPts := lastNHours(pts, w)
		if len(windowPts) >= 2 {
			page.Charts[w] = LineChart(windowPts, 700, 280)
		}
	}

	// Latest check and recent checks from typed history.
	page.LatestCheck, page.RecentChecks, page.RecentCount = extractCheckDetails(r.History)

	return page
}

// extractCheckDetails returns the latest check row and the last N-1 recent rows
// (newest first, excluding the latest) from the typed history slice.
func extractCheckDetails(history interface{}) (latest interface{}, recent interface{}, count int) {
	switch h := history.(type) {
	case []db.HTTPCheck:
		if len(h) == 0 {
			return nil, nil, 0
		}
		latest = &h[len(h)-1]
		reversed, n := reverseSkipFirst(h)
		return latest, reversed, n
	case []db.PingCheck:
		if len(h) == 0 {
			return nil, nil, 0
		}
		latest = &h[len(h)-1]
		reversed, n := reverseSkipFirst(h)
		return latest, reversed, n
	case []db.TCPCheck:
		if len(h) == 0 {
			return nil, nil, 0
		}
		latest = &h[len(h)-1]
		reversed, n := reverseSkipFirst(h)
		return latest, reversed, n
	case []db.DNSCheck:
		if len(h) == 0 {
			return nil, nil, 0
		}
		latest = &h[len(h)-1]
		reversed, n := reverseSkipFirst(h)
		return latest, reversed, n
	case []db.SSLCheck:
		if len(h) == 0 {
			return nil, nil, 0
		}
		latest = &h[len(h)-1]
		reversed, n := reverseSkipFirst(h)
		return latest, reversed, n
	}
	return nil, nil, 0
}

// reverseSkipFirst reverses a slice of any type, skipping the last element
// (the newest entry). Returns the reversed subset and its length.
func reverseSkipFirst[S ~[]E, E any](s S) (S, int) {
	if len(s) <= 1 {
		return nil, 0
	}
	n := len(s) - 1
	if n > maxRecentChecks {
		n = maxRecentChecks
	}
	reversed := make(S, n)
	for i := range n {
		reversed[i] = s[len(s)-2-i]
	}
	return reversed, n
}

// writeDetailPage renders a single resource detail page to disk.
func writeDetailPage(tmpl *template.Template, resourcesDir string, page ResourcePage) error {
	slugDir := filepath.Join(resourcesDir, page.Slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", slugDir, err)
	}

	outPath := filepath.Join(slugDir, "index.html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if err := tmpl.ExecuteTemplate(f, "base.html", page); err != nil {
		return fmt.Errorf("render detail %s: %w", page.Slug, err)
	}
	fmt.Printf("  Wrote %s\n", outPath)
	return nil
}

// lastNHours returns points within the last n hours from the end of the slice.
func lastNHours(pts []checkPoint, hours int) []checkPoint {
	if len(pts) == 0 {
		return nil
	}
	lastTS := pts[len(pts)-1].ts
	cutoff := ""
	if t, err := time.Parse("2006-01-02 15:04:05", lastTS); err == nil {
		cutoff = t.Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04:05")
	}
	if cutoff == "" {
		return pts
	}
	for i := range pts {
		if pts[i].ts >= cutoff {
			return pts[i:]
		}
	}
	return pts[len(pts)-1:]
}

// calcRespStats returns avg, min, max response times from points.
func calcRespStats(pts []checkPoint) (avg, min, max float64) {
	if len(pts) == 0 {
		return 0, 0, 0
	}
	min = pts[0].resp
	max = pts[0].resp
	sum := 0.0
	for _, p := range pts {
		sum += p.resp
		if p.resp < min {
			min = p.resp
		}
		if p.resp > max {
			max = p.resp
		}
	}
	return sum / float64(len(pts)), min, max
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
