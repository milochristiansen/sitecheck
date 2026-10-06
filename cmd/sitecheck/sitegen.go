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

	"sitecheck/core"
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

// RenderedCheck holds a pre-identified check with template dispatch info. When Elided
// is non-empty the entry is an interstitial marker for checks collapsed by elision
// (Data is nil).
type RenderedCheck struct {
	RowTemplateName  string
	BodyTemplateName string
	Data             interface{}
	Elided           string
	// SourceID is the primary key of the light row this entry represents. Empty
	// for elision markers. Used to hydrate only the rows that survive elision.
	SourceID string
}

// OutpostResource is a summary of a resource belonging to an outpost, for the outpost detail page.
type OutpostResource struct {
	Name       string
	Slug       string
	Pass       int // 2=pass, 1=degraded, 0=fail, -1=unknown
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

	// Charts: one SVG per chart window (keyed by window hours); display is hardcoded
	// to the 24h and 30-day charts.
	Charts         map[int]template.HTML
	DurationCharts map[int]template.HTML
	ChartWindows   []int

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
}

// Chart windows for the detail page, hardcoded: a 24h response-time chart and a 30-day
// chart. No longer configurable.
const (
	chartWindow24h = 24
	chartWindow30d = 720
)

// chartWindows lists the windows in display order.
var chartWindows = []int{chartWindow24h, chartWindow30d}

// Generate renders the static site into cfg.OutputDir — one subdirectory per site (implicit
// "default" first, extras sorted by name) — from the sorted slice of Results. History is not
// carried on Results: the src is queried lazily for exactly the data each page needs. Every
// site has the same internal layout; the per-level split lives in the card and detail
// templates, not the index.
func Generate(cfg *Config, results []SiteResult, src historySource) error {
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
		if err := generateSite(cfg, site, results, src, ir); err != nil {
			return err
		}
	}
	return nil
}

// generateSite renders one site (overview page, detail pages, and its own static/ copy).
func generateSite(cfg *Config, site Site, results []SiteResult, src historySource, ir *templateRenderer) error {
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

	resourceCards, err := buildCards(resourceResults, src)
	if err != nil {
		return fmt.Errorf("build resource cards for site %s: %w", site.Name, err)
	}
	outpostCards, err := buildCards(outpostResults, src)
	if err != nil {
		return fmt.Errorf("build outpost cards for site %s: %w", site.Name, err)
	}
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
		page, err := buildResourcePage(cfg, r, src)
		if err != nil {
			return fmt.Errorf("build detail page for %s %q: %w", resultKind(r), r.Slug, err)
		}
		if r.CheckType == "outpost" {
			page.Resources = outpostResources[r.Slug]
		}
		if err := writeDetailPage(cfg.TemplatesDir, resourcesDir, levelFor(site.Name, r), page); err != nil {
			return err
		}
	}
	return nil
}

// sparklinePoints is the number of most-recent checks drawn on a card sparkline.
// Points are spread evenly across the 300-unit sparkline canvas; the status dots are
// ~4 units wide, so more than ~74 points would fuse them into a band on the narrowest
// card. 70 is the round number just under that bound.
const sparklinePoints = 70

// buildCards builds the overview cards for a set of results. Instead of a full
// history slice, it pulls only the two narrow windows it needs: 24h of points for
// the uptime figure, and the last sparklinePoints points for the sparkline. Both
// are dropped as soon as the card is built.
func buildCards(results []SiteResult, src historySource) ([]ResourceCard, error) {
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
		if _, ok := src.Plugin(r.CheckType); ok {
			since24h := time.Now().UTC().Add(-24 * time.Hour)
			uptimePts, err := src.Points(r.CheckType, r.Slug, r.OutpostSlug, since24h, 0)
			if err != nil {
				return nil, fmt.Errorf("card uptime for %s/%s: %w", r.OutpostSlug, r.Slug, err)
			}
			card.Uptime24h = calcUptimePct(uptimePts)

			// Sparklines show the last sparklinePoints checks: a fixed count keeps the
			// density constant regardless of check cadence.
			sparkPts, err := src.Points(r.CheckType, r.Slug, r.OutpostSlug, time.Time{}, sparklinePoints)
			if err != nil {
				return nil, fmt.Errorf("card sparkline for %s/%s: %w", r.OutpostSlug, r.Slug, err)
			}
			card.Sparkline = Sparkline(sparkPts, 300, 30)
		}
		cards = append(cards, card)
	}
	return cards, nil
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
		case core.UNKNOWN:
			unknown++
		}
	}
	return
}

// buildResourcePage constructs a ResourcePage from a single Result. All history
// is fetched here, used, and dropped; only the rows that survive elision are
// hydrated with their large text fields.
func buildResourcePage(cfg *Config, r SiteResult, src historySource) (ResourcePage, error) {
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
		ChartWindows: chartWindows,
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
	p, hasPlugin := src.Plugin(r.CheckType)
	if !hasPlugin {
		return page, nil
	}

	// A single narrow point query feeds stats and both chart families. It carries
	// no large text columns, so it is cheap even for a 30-day window.
	since := time.Now().UTC().Add(-time.Duration(chartWindow30d) * time.Hour)
	pts, err := src.Points(r.CheckType, r.Slug, r.OutpostSlug, since, 0)
	if err != nil {
		return page, fmt.Errorf("history points for %s/%s: %w", r.OutpostSlug, r.Slug, err)
	}

	if len(pts) > 0 {
		page.TotalChecks = len(pts)
		page.AvgResponseMS, page.MinResponseMS, page.MaxResponseMS = calcRespStats(pts)
		page.Uptime24h = calcUptimePct(lastNHours(pts, 24))
		page.Uptime7d = calcUptimePct(lastNHours(pts, 7*24))
		page.Uptime30d = calcUptimePct(pts)
	}

	// Charts for the fixed windows. The x-axis is anchored to generation time so each
	// chart always shows the full window. Both charts ship in two sizes (page-width
	// and standard); CSS picks one per device. The 30d chart shows 8-hour averages.
	page.Charts = make(map[int]template.HTML)
	chartEnd := time.Now().UTC()
	for _, w := range chartWindows {
		windowPts := lastNHours(pts, w)
		if len(windowPts) < 2 {
			continue
		}
		start := chartEnd.Add(-time.Duration(w) * time.Hour)
		switch w {
		case chartWindow24h:
			page.Charts[w] = LineChartPair(windowPts, start, chartEnd)
		case chartWindow30d:
			page.Charts[w] = ThirtyDayChartPair(windowPts, start, chartEnd)
		}
	}

	// Duration stats and charts (outpost only), derived from the same point query.
	if r.CheckType == "outpost" {
		durPts := durationPoints(pts)
		if len(durPts) > 0 {
			page.DurationAvgMS, page.DurationMinMS, page.DurationMaxMS = calcRespStats(durPts)
			page.DurationCharts = make(map[int]template.HTML)
			for _, w := range chartWindows {
				windowPts := lastNHours(durPts, w)
				if len(windowPts) >= 2 {
					page.DurationCharts[w] = LineChart(windowPts, 700, 280, chartEnd.Add(-time.Duration(w)*time.Hour), chartEnd)
				}
			}
		}
	}

	// Latest check and recent checks. Light rows are streamed newest-first, elided
	// as they arrive, and only the surviving representatives are hydrated.
	rowName, bodyName := p.TemplateNames()
	builder := newElisionBuilder(rowName, bodyName)
	var latest *RenderedCheck
	var seen int
	err = src.EachRecentLight(r.CheckType, r.Slug, r.OutpostSlug, since, func(id string, row interface{}) error {
		seen++
		if seen == 1 {
			latest = &RenderedCheck{BodyTemplateName: bodyName, Data: row, SourceID: id}
			return nil
		}
		builder.Add(id, row)
		return nil
	})
	if err != nil {
		return page, fmt.Errorf("recent checks for %s/%s: %w", r.OutpostSlug, r.Slug, err)
	}
	page.LatestCheck = latest
	page.RecentChecks = builder.Finish()
	if seen > 0 {
		page.RecentCount = seen - 1
	}

	if p.NeedsHydration() {
		if page.LatestCheck != nil {
			full, err := src.LoadFull(r.CheckType, page.LatestCheck.SourceID)
			if err != nil {
				return page, fmt.Errorf("hydrate latest %s/%s: %w", r.OutpostSlug, r.Slug, err)
			}
			page.LatestCheck.Data = full
		}
		for i := range page.RecentChecks {
			if page.RecentChecks[i].SourceID == "" {
				continue
			}
			full, err := src.LoadFull(r.CheckType, page.RecentChecks[i].SourceID)
			if err != nil {
				return page, fmt.Errorf("hydrate recent %s/%s: %w", r.OutpostSlug, r.Slug, err)
			}
			page.RecentChecks[i].Data = full
		}
	}

	return page, nil
}

// durationPoints derives run-time points from the shared point slice. Only the
// outpost check type consumes these.
func durationPoints(pts []core.CheckPoint) []core.CheckPoint {
	out := make([]core.CheckPoint, 0, len(pts))
	for _, p := range pts {
		out = append(out, core.CheckPoint{Pass: p.Pass, Resp: p.Duration, TS: p.TS})
	}
	return out
}

// templateRenderer holds a fully parsed template set so that renderCheck can close over it.
type templateRenderer struct {
	tmpl *template.Template
}

// checksSimilar reports whether two checks of the same type are the same event apart
// from run timing. Large text fields are compared by hash (see core.Similar), which
// lets the caller pass light rows that omit those columns.
func checksSimilar(a, b interface{}) bool {
	return core.Similar(a, b)
}

// elisionBuilder collapses runs of adjacent similar checks into their newest
// representative plus an interstitial marker. It consumes light rows one at a time,
// so the caller never holds more than the current run plus the emitted output.
type elisionBuilder struct {
	rowName  string
	bodyName string
	have     bool
	cur      interface{}
	curID    string
	curPass  int
	runs     int
	out      []RenderedCheck
}

func newElisionBuilder(rowName, bodyName string) *elisionBuilder {
	return &elisionBuilder{rowName: rowName, bodyName: bodyName}
}

// Add feeds one light row (newest-first order) into the builder.
func (b *elisionBuilder) Add(id string, row interface{}) {
	if !b.have {
		b.cur, b.curID, b.curPass, b.runs, b.have = row, id, passOf(row), 1, true
		return
	}
	if checksSimilar(b.cur, row) {
		b.runs++
		return
	}
	b.flush()
	b.cur, b.curID, b.curPass, b.runs, b.have = row, id, passOf(row), 1, true
}

// flush emits the current run's representative and, if the run collapsed more than
// one check, an elision marker.
func (b *elisionBuilder) flush() {
	if !b.have {
		return
	}
	b.out = append(b.out, RenderedCheck{
		RowTemplateName:  b.rowName,
		BodyTemplateName: b.bodyName,
		Data:             b.cur,
		SourceID:         b.curID,
	})
	if b.runs > 1 {
		b.out = append(b.out, RenderedCheck{
			Elided: fmt.Sprintf("(%d similar %s checks elided)", b.runs-1, passName(b.curPass)),
		})
	}
	b.have = false
}

// Finish flushes the final run and returns the rendered rows.
func (b *elisionBuilder) Finish() []RenderedCheck {
	b.flush()
	return b.out
}

// elideRecentChecks is the slice-based form of the builder, kept for tests and
// callers that already hold a newest-first slice.
func elideRecentChecks(checks interface{}, rowName, bodyName string) []RenderedCheck {
	rv := reflect.ValueOf(checks)
	b := newElisionBuilder(rowName, bodyName)
	for i := 0; i < rv.Len(); i++ {
		row := rv.Index(i).Interface()
		b.Add(rowID(row), row)
	}
	return b.Finish()
}

// passOf reads the Pass field from a typed check row.
func passOf(row interface{}) int {
	f := reflect.ValueOf(row).FieldByName("Pass")
	if !f.IsValid() {
		return 0
	}
	return int(f.Int())
}

// rowID renders a typed check row's primary key as a string. Most check tables use
// an integer id; exec uses a TEXT id.
func rowID(row interface{}) string {
	f := reflect.ValueOf(row).FieldByName("ID")
	if !f.IsValid() {
		return ""
	}
	switch f.Kind() {
	case reflect.String:
		return f.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", f.Int())
	default:
		return ""
	}
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

// detailRenderers caches parsed detail template sets per templatesDir+level;
// sitecheck renders many pages per level in a single run.
var detailRenderers = map[string]*templateRenderer{}

// detailRenderer parses (once per templatesDir+level) the page template from
// templates/<level>/resource.html and the check-type row/body templates from
// templates/<level>/checks/*.html. The checks templates are optional — a level
// whose resource.html never invokes renderCheck (e.g. basic) simply omits the
// checks/ directory.
func detailRenderer(templatesDir, level string) (*templateRenderer, error) {
	key := templatesDir + "/" + level
	if tr, ok := detailRenderers[key]; ok {
		return tr, nil
	}

	allFiles := []string{
		filepath.Join(templatesDir, "base.html"),
		filepath.Join(templatesDir, level, "resource.html"),
	}
	files, err := filepath.Glob(filepath.Join(templatesDir, level, "checks", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("glob check templates: %w", err)
	}
	allFiles = append(allFiles, files...)

	tr := &templateRenderer{}
	tmpl := template.New("").Funcs(template.FuncMap{
		"renderCheck":      tr.renderCheck,
		"statusClass":      statusClass,
		"passName":         passName,
		"formatPct":        formatPct,
		"formatDuration":   formatDuration,
		"formatDurationMS": formatDurationMS,
		"dict":             dict,
		"windowLabel":      windowLabel,
	})
	if _, err := tmpl.ParseFiles(allFiles...); err != nil {
		return nil, fmt.Errorf("parse detail templates: %w", err)
	}
	tr.tmpl = tmpl
	detailRenderers[key] = tr
	return tr, nil
}

// writeDetailPage renders a single resource detail page to disk.
func writeDetailPage(templatesDir, resourcesDir, level string, page ResourcePage) error {
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", resourcesDir, err)
	}

	tr, err := detailRenderer(templatesDir, level)
	if err != nil {
		return err
	}

	outPath := filepath.Join(resourcesDir, page.Slug+".html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if err := tr.tmpl.ExecuteTemplate(f, "base.html", page); err != nil {
		return fmt.Errorf("render %s: %w", outPath, err)
	}
	fmt.Printf("  Wrote %s\n", outPath)
	return nil
}

// lastNHours returns points within the last n hours from the end of the slice.
func lastNHours(pts []core.CheckPoint, hours int) []core.CheckPoint {
	if len(pts) == 0 {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UTC().Format("2006-01-02 15:04:05")
	for i := range pts {
		if pts[i].TS >= cutoff {
			return pts[i:]
		}
	}
	return pts[len(pts)-1:]
}

// lastN returns the last n points from the end of the slice, or all of them when
// there are fewer than n. Points must be in chronological order (oldest first).
func lastN(pts []core.CheckPoint, n int) []core.CheckPoint {
	if len(pts) <= n {
		return pts
	}
	return pts[len(pts)-n:]
}

// calcRespStats returns avg, min, max response times from points.
func calcRespStats(pts []core.CheckPoint) (avg, min, max float64) {
	if len(pts) == 0 {
		return 0, 0, 0
	}
	min = pts[0].Resp
	max = pts[0].Resp
	var sum float64
	for _, p := range pts {
		sum += p.Resp
		if p.Resp < min {
			min = p.Resp
		}
		if p.Resp > max {
			max = p.Resp
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
