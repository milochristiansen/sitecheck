package main

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sitecheck/checktypes/http"
)

func TestCountStatuses(t *testing.T) {
	t.Run("empty returns all zeros", func(t *testing.T) {
		up, degraded, down, unknown := countStatuses(nil)
		if up != 0 || degraded != 0 || down != 0 || unknown != 0 {
			t.Errorf("nil: got (%d,%d,%d,%d), want (0,0,0,0)", up, degraded, down, unknown)
		}
		up, degraded, down, unknown = countStatuses([]ResourceCard{})
		if up != 0 || degraded != 0 || down != 0 || unknown != 0 {
			t.Errorf("empty slice: got (%d,%d,%d,%d), want (0,0,0,0)", up, degraded, down, unknown)
		}
	})

	t.Run("all PASS returns (n,0,0,0)", func(t *testing.T) {
		cards := []ResourceCard{
			{Pass: 2},
			{Pass: 2},
			{Pass: 2},
		}
		up, degraded, down, unknown := countStatuses(cards)
		if up != 3 || degraded != 0 || down != 0 || unknown != 0 {
			t.Errorf("all PASS: got (%d,%d,%d,%d), want (3,0,0,0)", up, degraded, down, unknown)
		}
	})

	t.Run("mixed returns correct counts", func(t *testing.T) {
		cards := []ResourceCard{
			{Pass: 2}, // up
			{Pass: 1}, // degraded
			{Pass: 0}, // down
			{Pass: 2}, // up
			{Pass: -1}, // unknown
			{Pass: 0}, // down
		}
		up, degraded, down, unknown := countStatuses(cards)
		if up != 2 || degraded != 1 || down != 2 || unknown != 1 {
			t.Errorf("mixed: got (%d,%d,%d,%d), want (2,1,2,1)", up, degraded, down, unknown)
		}
	})

	t.Run("unknown cards count correctly", func(t *testing.T) {
		cards := []ResourceCard{
			{Pass: -1},
			{Pass: -1},
			{Pass: 2},
		}
		up, degraded, down, unknown := countStatuses(cards)
		if up != 1 || degraded != 0 || down != 0 || unknown != 2 {
			t.Errorf("with unknown: got (%d,%d,%d,%d), want (1,0,0,2)", up, degraded, down, unknown)
		}
	})
}

func TestLastNHours(t *testing.T) {
	now := time.Now().UTC()
	fmt := "2006-01-02 15:04:05"

	t.Run("empty returns nil", func(t *testing.T) {
		result := lastNHours(nil, 24)
		if result != nil {
			t.Errorf("nil: got non-nil")
		}
		result = lastNHours([]checkPoint{}, 24)
		if result != nil {
			t.Errorf("empty slice: got non-nil")
		}
	})

	t.Run("all points within window", func(t *testing.T) {
		pts := []checkPoint{
			{ts: now.Add(-1 * time.Hour).Format(fmt)},
			{ts: now.Add(-2 * time.Hour).Format(fmt)},
			{ts: now.Add(-3 * time.Hour).Format(fmt)},
		}
		result := lastNHours(pts, 24)
		if len(result) != 3 {
			t.Errorf("all within: got %d points, want 3", len(result))
		}
	})

	t.Run("some points outside window", func(t *testing.T) {
		pts := []checkPoint{
			{ts: now.Add(-48 * time.Hour).Format(fmt)},
			{ts: now.Add(-36 * time.Hour).Format(fmt)},
			{ts: now.Add(-2 * time.Hour).Format(fmt)},
			{ts: now.Add(-1 * time.Hour).Format(fmt)},
		}
		result := lastNHours(pts, 12)
		if len(result) != 2 {
			t.Errorf("some outside: got %d points, want 2", len(result))
		}
		// Verify the returned points are the recent ones
		if result[0].ts != pts[2].ts || result[1].ts != pts[3].ts {
			t.Errorf("some outside: returned wrong points")
		}
	})

	t.Run("all points outside window returns last point", func(t *testing.T) {
		pts := []checkPoint{
			{ts: now.Add(-72 * time.Hour).Format(fmt)},
			{ts: now.Add(-48 * time.Hour).Format(fmt)},
		}
		result := lastNHours(pts, 24)
		if len(result) != 1 {
			t.Errorf("all outside: got %d points, want 1", len(result))
		}
		if result[0].ts != pts[1].ts {
			t.Errorf("all outside: got ts=%q, want last point ts=%q", result[0].ts, pts[1].ts)
		}
	})
}

func TestCalcRespStats(t *testing.T) {
	const tol = 1e-9

	t.Run("empty returns (0,0,0)", func(t *testing.T) {
		avg, min, max := calcRespStats(nil)
		if avg != 0 || min != 0 || max != 0 {
			t.Errorf("nil: got (%f,%f,%f), want (0,0,0)", avg, min, max)
		}
		avg, min, max = calcRespStats([]checkPoint{})
		if avg != 0 || min != 0 || max != 0 {
			t.Errorf("empty slice: got (%f,%f,%f), want (0,0,0)", avg, min, max)
		}
	})

	t.Run("single point returns (v,v,v)", func(t *testing.T) {
		pts := []checkPoint{{resp: 42.5}}
		avg, min, max := calcRespStats(pts)
		if avg < 42.5-tol || avg > 42.5+tol {
			t.Errorf("single avg: got %f, want 42.5", avg)
		}
		if min < 42.5-tol || min > 42.5+tol {
			t.Errorf("single min: got %f, want 42.5", min)
		}
		if max < 42.5-tol || max > 42.5+tol {
			t.Errorf("single max: got %f, want 42.5", max)
		}
	})

	t.Run("multiple points returns correct stats", func(t *testing.T) {
		pts := []checkPoint{
			{resp: 10.0},
			{resp: 20.0},
			{resp: 30.0},
		}
		avg, min, max := calcRespStats(pts)
		if avg < 20.0-tol || avg > 20.0+tol {
			t.Errorf("avg: got %f, want 20.0", avg)
		}
		if min < 10.0-tol || min > 10.0+tol {
			t.Errorf("min: got %f, want 10.0", min)
		}
		if max < 30.0-tol || max > 30.0+tol {
			t.Errorf("max: got %f, want 30.0", max)
		}
	})

	t.Run("unsorted points still give correct min/max", func(t *testing.T) {
		pts := []checkPoint{
			{resp: 50.0},
			{resp: 10.0},
			{resp: 30.0},
		}
		avg, min, max := calcRespStats(pts)
		if avg < 30.0-tol || avg > 30.0+tol {
			t.Errorf("avg: got %f, want 30.0", avg)
		}
		if min < 10.0-tol || min > 10.0+tol {
			t.Errorf("min: got %f, want 10.0", min)
		}
		if max < 50.0-tol || max > 50.0+tol {
			t.Errorf("max: got %f, want 50.0", max)
		}
	})
}

func TestBuildCards(t *testing.T) {
	t.Run("empty results returns empty cards", func(t *testing.T) {
		cards := buildCards(nil)
		if len(cards) != 0 {
			t.Errorf("nil: got %d cards, want 0", len(cards))
		}
		cards = buildCards([]SiteResult{})
		if len(cards) != 0 {
			t.Errorf("empty slice: got %d cards, want 0", len(cards))
		}
	})

	t.Run("non-outpost slug is outpostSlug-checkSlug", func(t *testing.T) {
		results := []SiteResult{
			{Slug: "http", Name: "HTTP Check", CheckType: "http", OutpostSlug: "main", OutpostName: "Main Outpost"},
		}
		cards := buildCards(results)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		if cards[0].Slug != "main-http" {
			t.Errorf("slug = %q, want %q", cards[0].Slug, "main-http")
		}
		if cards[0].Name != "HTTP Check" {
			t.Errorf("name = %q, want %q", cards[0].Name, "HTTP Check")
		}
		if cards[0].OutpostSlug != "main" {
			t.Errorf("OutpostSlug = %q, want %q", cards[0].OutpostSlug, "main")
		}
		if cards[0].OutpostName != "Main Outpost" {
			t.Errorf("OutpostName = %q, want %q", cards[0].OutpostName, "Main Outpost")
		}
	})

	t.Run("outpost slug is just slug", func(t *testing.T) {
		results := []SiteResult{
			{Slug: "my-outpost", Name: "My Outpost", CheckType: "outpost"},
		}
		cards := buildCards(results)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		if cards[0].Slug != "my-outpost" {
			t.Errorf("slug = %q, want %q", cards[0].Slug, "my-outpost")
		}
		if cards[0].CheckType != "outpost" {
			t.Errorf("CheckType = %q, want %q", cards[0].CheckType, "outpost")
		}
	})

	t.Run("Err populates empty FailReason", func(t *testing.T) {
		results := []SiteResult{
			{Slug: "http", CheckType: "http", Err: "connection refused", OutpostSlug: "main"},
		}
		cards := buildCards(results)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		if cards[0].FailReason != "connection refused" {
			t.Errorf("FailReason = %q, want %q", cards[0].FailReason, "connection refused")
		}
	})

	t.Run("existing FailReason not overridden by Err", func(t *testing.T) {
		results := []SiteResult{
			{Slug: "http", CheckType: "http", FailReason: "timeout", Err: "connection refused", OutpostSlug: "main"},
		}
		cards := buildCards(results)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		if cards[0].FailReason != "timeout" {
			t.Errorf("FailReason = %q, want %q", cards[0].FailReason, "timeout")
		}
	})

	t.Run("no Err or FailReason leaves FailReason empty", func(t *testing.T) {
		results := []SiteResult{
			{Slug: "http", CheckType: "http", OutpostSlug: "main"},
		}
		cards := buildCards(results)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		if cards[0].FailReason != "" {
			t.Errorf("FailReason = %q, want empty", cards[0].FailReason)
		}
	})
}

func TestBuildCardsSparklineCappedTo24h(t *testing.T) {
	// History spanning more than a day: the card sparkline must only include the last 24h.
	// Sparkline renders one <circle> per point, so 3 rows total → 2 in the window.
	now := time.Now().UTC()
	ts := func(h int) string { return now.Add(-time.Duration(h) * time.Hour).Format("2006-01-02 15:04:05") }
	hist := []http.HTTPCheck{
		{Timestamp: ts(48), ResponseTimeMS: 5},
		{Timestamp: ts(6), ResponseTimeMS: 10},
		{Timestamp: ts(1), ResponseTimeMS: 20},
	}
	results := []SiteResult{{Slug: "http", Name: "HTTP Check", CheckType: "http", OutpostSlug: "main", History: hist}}
	cards := buildCards(results)
	if cards[0].Sparkline == template.HTML("") {
		t.Fatal("sparkline empty, want 2 points rendered")
	}
	if n := strings.Count(string(cards[0].Sparkline), "<circle"); n != 2 {
		t.Errorf("sparkline has %d points, want 2 (24h cap should exclude the 48h-old point)", n)
	}
}

// renderCardTestTemplates writes a minimal template set (base + index + per-level cards) and
// returns the dir, mirroring how Generate parses the real set.
func renderCardTestTemplates(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("base.html", "<html><head><title>{{.Title}}</title></head><body>{{template \"content\" .}}</body></html>")
	write("index.html", "{{define \"content\"}}<div class=\"resource-grid\">{{range .Entries}}{{renderCard .Level .}}{{end}}</div>{{end}}")
	write(filepath.Join("full", "card.html"), "{{define \"card-full\"}}FULL:{{.Name}}:{{.Pass}}{{end}}")
	write(filepath.Join("basic", "card.html"), "{{define \"card-basic\"}}BASIC:{{.Name}}:{{.Pass}}{{end}}")
	return dir
}

func TestRenderCard(t *testing.T) {
	dir := renderCardTestTemplates(t)
	ir := &templateRenderer{}
	funcs := tmplFuncs()
	funcs["renderCard"] = ir.renderCard
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFiles(
		filepath.Join(dir, "base.html"),
		filepath.Join(dir, "index.html"),
		filepath.Join(dir, "full", "card.html"),
		filepath.Join(dir, "basic", "card.html"),
	))
	ir.tmpl = tmpl
	card := ResourceCard{Name: "X", Pass: 2}

	full, err := ir.renderCard("full", card)
	if err != nil {
		t.Fatalf("renderCard(full): %v", err)
	}
	if full != "FULL:X:2" {
		t.Errorf("renderCard(full) = %q, want %q", full, "FULL:X:2")
	}

	basic, err := ir.renderCard("basic", card)
	if err != nil {
		t.Fatalf("renderCard(basic): %v", err)
	}
	if basic != "BASIC:X:2" {
		t.Errorf("renderCard(basic) = %q, want %q", basic, "BASIC:X:2")
	}
}

func TestRenderCardIndexMixedLevels(t *testing.T) {
	dir := renderCardTestTemplates(t)
	ir := &templateRenderer{}
	funcs := tmplFuncs()
	funcs["renderCard"] = ir.renderCard
	indexFiles := []string{
		filepath.Join(dir, "base.html"),
		filepath.Join(dir, "index.html"),
	}
	cardFiles, err := filepath.Glob(filepath.Join(dir, "*", "card.html"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	indexFiles = append(indexFiles, cardFiles...)
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFiles(indexFiles...))
	ir.tmpl = tmpl

	data := IndexData{
		Title:   "T",
		Entries: []ResourceCard{{Name: "A", Pass: 2, Level: "full"}, {Name: "B", Pass: 0, Level: "basic"}},
	}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "FULL:A:2") {
		t.Errorf("output missing full card: %s", out)
	}
	if !strings.Contains(out, "BASIC:B:0") {
		t.Errorf("output missing basic card: %s", out)
	}
}
