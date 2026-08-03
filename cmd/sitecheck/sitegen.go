package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"sitecheck/checktypes/registry"
)
// IndexData is the template data for index.html.
type IndexData struct {
	Title         string
	SiteTitle     string
	Generated     string
	StaticPrefix  string
	Entries       []ResourceCard
	Outposts      []ResourceCard
	UpCount       int
	DegradedCount int
	DownCount     int
	UnknownCount  int
}

// ResourceCard holds per-resource display data for the overview page.
type ResourceCard struct {
	Slug        string
	Name        string
	CheckType   string
	Pass        int // 2=pass, 1=degraded, 0=fail, -1=no data
	ResponseMS  float64
	Uptime24h   float64
	Sparkline   template.HTML
	FailReason  string
	OutpostSlug string
	OutpostName string
	Level       string // detail level of this result within the site being rendered
}

// RenderedCheck holds a pre-identified check with template dispatch info.
type RenderedCheck struct {
	RowTemplateName  string
	BodyTemplateName string
	Data             interface{}
}

// OutpostResource is a summary of a resource belonging to an outpost, for the outpost detail page.
type OutpostResource struct {
	Name       string
	Slug       string
	Pass       int     // 2=pass, 1=degraded, 0=fail, -1=unknown
	CheckType  string
	ResponseMS float64 // response time of the member's latest check (basic level only)
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
	OutpostSlug  string
	OutpostName  string

	// Stats (response time)
	Uptime24h     float64
	Uptime7d      float64
	Uptime30d     float64
	AvgResponseMS float64
	MinResponseMS float64
	MaxResponseMS float64
	TotalChecks   int

	// Duration stats (run time, outpost only)
	DurationAvgMS float64
	DurationMinMS float64
	DurationMaxMS float64

	// Charts: one SVG per graph window (keyed by window hours)
	Charts         map[int]template.HTML
	DurationCharts map[int]template.HTML
	GraphWindows   []int

	// Latest check row for display.
	LatestCheck *RenderedCheck

	// Recent checks for the collapsible table.
	RecentChecks []RenderedCheck
	RecentCount  int

	// Resources belonging to this outpost (outpost detail page only).
	Resources []OutpostResource
}

// SiteResult carries the data sitegen needs for a single resource.
type SiteResult struct {
	Slug        string
	Name        string
	Desc        string
	CheckType   string
	Pass        int
	FailReason  string
	ResponseMS  float64
	Err         string
	OutpostSlug string
	OutpostName string
	Sites       map[string]string // site name → detail level
	History     interface{}       // typed DB check slice, populated by caller
}

const maxRecentChecks = 15

// Generate renders the static site into cfg.OutputDir — one subdirectory per site (implicit
// "default" first, extras sorted by name) — from the sorted slice of Results (each with
// History populated by the collector). Every site has the same internal layout; the per-level
// split lives in the card and detail templates, not the index.
func Generate(cfg *Config, results []SiteResult) error {
	sites, err := planSites(cfg, results)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Parse the shared index template set once: base + index + every level's card template.
	// The index draws each card through renderCard(card.Level, card), dispatching to the card
	// template named for the result's level, so one page can mix levels.
	ir := &templateRenderer{}
	funcs := tmplFuncs()
	funcs["renderCard"] = ir.renderCard
	indexFiles := []string{
		filepath.Join(cfg.TemplatesDir, "base.html"),
		filepath.Join(cfg.TemplatesDir, "index.html"),
	}
	cardFiles, err := filepath.Glob(filepath.Join(cfg.TemplatesDir, "*", "card.html"))
	if err != nil {
		return fmt.Errorf("glob card templates: %w", err)
	}
	indexFiles = append(indexFiles, cardFiles...)
	indexTmpl, err := template.New("").Funcs(funcs).ParseFiles(indexFiles...)
	if err != nil {
		return fmt.Errorf("parse index templates: %w", err)
	}
	ir.tmpl = indexTmpl

	for _, site := range sites {
		if err := generateSite(cfg, site, results, ir); err != nil {
			return err
		}
	}
	return nil
}

// generateSite renders one site (overview page, detail pages, and its own static/ copy).
func generateSite(cfg *Config, site Site, results []SiteResult, ir *templateRenderer) error {
	siteDir := filepath.Join(cfg.OutputDir, site.Name)
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", siteDir, err)
	}
	if err := copyDir(cfg.StaticDir, filepath.Join(siteDir, "static")); err != nil {
		return fmt.Errorf("copy static for site %s: %w", site.Name, err)
	}

	resourceResults, outpostResults := siteMembers(site.Name, results)

	// Outpost detail pages list exactly this site's resources.
	outpostResources := make(map[string][]OutpostResource)
	for _, r := range resourceResults {
		outpostResources[r.OutpostSlug] = append(outpostResources[r.OutpostSlug], OutpostResource{
			Name:       r.Name,
			Slug:       r.OutpostSlug + "-" + r.Slug,
			Pass:       r.Pass,
			CheckType:  r.CheckType,
			ResponseMS: r.ResponseMS,
		})
	}

	resourceCards := buildCards(resourceResults)
	outpostCards := buildCards(outpostResults)
	for i := range resourceCards {
		resourceCards[i].Level = levelFor(site.Name, resourceResults[i])
	}
	for i := range outpostCards {
		outpostCards[i].Level = levelFor(site.Name, outpostResults[i])
	}
	up, deg, down, unknown := countStatuses(resourceCards)

	title := cfg.SiteTitle + " — Overview"
	if site.Name != defaultSiteName {
		title = cfg.SiteTitle + " — " + site.Name
	}
	data := IndexData{
		Title:         title,
		SiteTitle:     cfg.SiteTitle,
		Generated:     time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		StaticPrefix:  "static/",
		Entries:       resourceCards,
		Outposts:      outpostCards,
		UpCount:       up,
		DegradedCount: deg,
		DownCount:     down,
		UnknownCount:  unknown,
	}

	outPath := filepath.Join(siteDir, "index.html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	if err := ir.tmpl.ExecuteTemplate(f, "base.html", data); err != nil {
		f.Close()
		return fmt.Errorf("render index %s: %w", outPath, err)
	}
	f.Close()
	fmt.Printf("  Wrote %s\n", outPath)

	// Detail pages: every member result renders at its own level. basic and full receive the
	// same ResourcePage data; the basic template simply renders less of it.
	resourcesDir := filepath.Join(siteDir, "resources")
	members := append(resourceResults, outpostResults...)
	for _, r := range members {
		page := buildResourcePage(cfg, r)
		if r.CheckType == "outpost" {
			page.Resources = outpostResources[r.Slug]
		}
		if err := writeDetailPage(cfg.TemplatesDir, resourcesDir, levelFor(site.Name, r), page); err != nil {
			return err
		}
	}
	return nil
}


func buildCards(results []SiteResult) []ResourceCard {
	cards := make([]ResourceCard, 0, len(results))
	for _, r := range results {
		slug := r.Slug
		if r.CheckType != "outpost" {
			slug = r.OutpostSlug + "-" + r.Slug
		}
		card := ResourceCard{
			Slug:        slug,
			Name:        r.Name,
			CheckType:   r.CheckType,
			Pass:        r.Pass,
			ResponseMS:  r.ResponseMS,
			FailReason:  r.FailReason,
			OutpostSlug: r.OutpostSlug,
			OutpostName: r.OutpostName,
		}
		if r.Err != "" && card.FailReason == "" {
			card.FailReason = r.Err
		}
		if r.History != nil {
			p, ok := registry.ByName(r.CheckType)
			if ok {
				pts := extractPoints(r.History, p)
				card.Uptime24h = calcUptimePct(lastNHours(pts, 24))
				card.Sparkline = Sparkline(pts, 120, 30)
			}
		}
		cards = append(cards, card)
	}
	return cards
}

func countStatuses(cards []ResourceCard) (up, degraded, down, unknown int) {
	for _, c := range cards {
		switch c.Pass {
		case 2:
			up++
		case 1:
			degraded++
		case 0:
			down++
		case -1:
			unknown++
		}
	}
	return
}

// buildResourcePage constructs a ResourcePage from a single Result.
func buildResourcePage(cfg *Config, r SiteResult) ResourcePage {
	slug := r.Slug
	if r.CheckType != "outpost" {
		slug = r.OutpostSlug + "-" + r.Slug
	}
	page := ResourcePage{
		Title:        r.Name + " — " + cfg.SiteTitle,
		SiteTitle:    cfg.SiteTitle,
		Generated:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		StaticPrefix: "../static/",
		Slug:         slug,
		Name:         r.Name,
		Description:  r.Desc,
		GraphWindows: cfg.GraphWindows,
		OutpostSlug:  r.OutpostSlug,
		OutpostName:  r.OutpostName,
	}

	page.CheckType = r.CheckType
	page.Pass = r.Pass
	page.FailReason = r.FailReason
	page.ResponseMS = r.ResponseMS
	if r.Err != "" && page.FailReason == "" {
		page.FailReason = r.Err
	}

	// Look up the plugin for this check type.
	p, hasPlugin := registry.ByName(r.CheckType)

	if hasPlugin {
		pts := extractPoints(r.History, p)
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

		// Duration stats and charts (plugin-driven).
		durPts := extractDurationPoints(r.History, p)
		if len(durPts) > 0 {
			page.DurationAvgMS, page.DurationMinMS, page.DurationMaxMS = calcRespStats(durPts)
			page.DurationCharts = make(map[int]template.HTML)
			for _, w := range cfg.GraphWindows {
				windowPts := lastNHours(durPts, w)
				if len(windowPts) >= 2 {
					page.DurationCharts[w] = LineChart(windowPts, 700, 280)
				}
			}
		}

		// Latest check and recent checks via plugin.
		if r.History != nil {
			latest, recent, count := p.LatestRecent(r.History, maxRecentChecks)
			rowName, bodyName := p.TemplateNames()
			if latest != nil {
				page.LatestCheck = &RenderedCheck{
					BodyTemplateName: bodyName,
					Data:             latest,
				}
			}
			if recent != nil && count > 0 {
				page.RecentCount = count
				rv := reflect.ValueOf(recent)
				page.RecentChecks = make([]RenderedCheck, 0, rv.Len())
				for i := 0; i < rv.Len(); i++ {
					page.RecentChecks = append(page.RecentChecks, RenderedCheck{
						RowTemplateName:  rowName,
						BodyTemplateName: bodyName,
						Data:             rv.Index(i).Interface(),
					})
				}
			}
		}
	}

	return page
}

// templateRenderer holds a fully parsed template set so that renderCheck can close over it.
type templateRenderer struct {
	tmpl *template.Template
}

func (tr *templateRenderer) renderCheck(name string, data interface{}) (template.HTML, error) {
	var buf bytes.Buffer
	if err := tr.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// renderCard dispatches an overview card to the card template named for its level
// (templates/<level>/card.html defines {{define "card-<level>"}}).
func (tr *templateRenderer) renderCard(level string, data interface{}) (template.HTML, error) {
	var buf bytes.Buffer
	if err := tr.tmpl.ExecuteTemplate(&buf, "card-"+level, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// writeDetailPage renders a single resource detail page to disk. It creates its own template set
// with renderCheck included; the page template comes from templates/<level>/resource.html and
// the check-type row/body templates from templates/<level>/checks/*.html. The checks templates
// are per-level and optional — a level whose resource.html never invokes renderCheck (e.g.
// basic) simply omits the checks/ directory.
func writeDetailPage(templatesDir, resourcesDir, level string, page ResourcePage) error {
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", resourcesDir, err)
	}

	// Collect all template files including this level's check type templates.
	allFiles := []string{
		filepath.Join(templatesDir, "base.html"),
		filepath.Join(templatesDir, level, "resource.html"),
	}
	files, err := filepath.Glob(filepath.Join(templatesDir, level, "checks", "*.html"))
	if err != nil {
		return fmt.Errorf("glob check templates: %w", err)
	}
	allFiles = append(allFiles, files...)


	// Create template with renderCheck function closure.
	tr := &templateRenderer{}
	tmpl := template.New("").Funcs(template.FuncMap{
		"renderCheck":      tr.renderCheck,
		"statusClass":      statusClass,
		"passName":         passName,
		"formatPct":        formatPct,
		"formatDuration":   formatDuration,
		"formatDurationMS": formatDurationMS,
		"dict":             dict,
	})
	if _, err := tmpl.ParseFiles(allFiles...); err != nil {
		return fmt.Errorf("parse detail templates: %w", err)
	}
	tr.tmpl = tmpl

	outPath := filepath.Join(resourcesDir, page.Slug+".html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if err := tmpl.ExecuteTemplate(f, "base.html", page); err != nil {
		return fmt.Errorf("render %s: %w", outPath, err)
	}
	fmt.Printf("  Wrote %s\n", outPath)
	return nil
}


// lastNHours returns points within the last n hours from the end of the slice.
func lastNHours(pts []checkPoint, hours int) []checkPoint {
	if len(pts) == 0 {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UTC().Format("2006-01-02 15:04:05")
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
	var sum float64
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
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = io.Copy(d, s)
	return err
}
