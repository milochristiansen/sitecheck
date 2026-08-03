package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// testTemplatesDir builds a templates dir with full/ and basic/ levels (card.html +
// resource.html each) and returns its path.
func testTemplatesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, lvl := range []string{"full", "basic"} {
		if err := os.MkdirAll(filepath.Join(dir, lvl), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"card.html", "resource.html"} {
			if err := os.WriteFile(filepath.Join(dir, lvl, f), []byte("{{define \""+lvl+"\"}}x{{end}}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

func testCfg(t *testing.T) *Config {
	t.Helper()
	return &Config{TemplatesDir: testTemplatesDir(t)}
}

func TestPlanSites(t *testing.T) {
	t.Run("no memberships yields just default", func(t *testing.T) {
		cfg := testCfg(t)
		sites, err := planSites(cfg, nil)
		if err != nil {
			t.Fatalf("planSites: %v", err)
		}
		if len(sites) != 1 || sites[0].Name != "default" {
			t.Errorf("sites = %+v, want [default]", sites)
		}
	})

	t.Run("union of extras, default first, sorted", func(t *testing.T) {
		cfg := testCfg(t)
		results := []SiteResult{
			{Slug: "a", CheckType: "http", Sites: map[string]string{"zeta": "basic"}},
			{Slug: "b", CheckType: "http", Sites: map[string]string{"alpha": "basic", "default": "basic"}},
			{Slug: "c", CheckType: "http", Sites: map[string]string{"zeta": "full"}},
		}
		sites, err := planSites(cfg, results)
		if err != nil {
			t.Fatalf("planSites: %v", err)
		}
		var names []string
		for _, s := range sites {
			names = append(names, s.Name)
		}
		want := []string{"default", "alpha", "zeta"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("sites = %v, want %v", names, want)
		}
	})

	t.Run("invalid site name errors", func(t *testing.T) {
		cfg := testCfg(t)
		results := []SiteResult{
			{Slug: "a", CheckType: "http", Sites: map[string]string{"../evil": "basic"}},
		}
		if _, err := planSites(cfg, results); err == nil {
			t.Error("planSites: expected error for invalid site name")
		}
	})

	t.Run("unknown level errors naming resource and known levels", func(t *testing.T) {
		cfg := testCfg(t)
		results := []SiteResult{
			{Slug: "a", CheckType: "http", Sites: map[string]string{"internal": "ultra"}},
		}
		_, err := planSites(cfg, results)
		if err == nil {
			t.Fatal("planSites: expected error for unknown level")
		}
		for _, want := range []string{`resource "a"`, `"ultra"`, `"full"`, `"basic"`} {
			if !containsStr(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err.Error(), want)
			}
		}
	})

	t.Run("outpost declarations never create sites", func(t *testing.T) {
		cfg := testCfg(t)
		results := []SiteResult{
			{Slug: "op", CheckType: "outpost", Sites: map[string]string{"internal": "basic"}},
		}
		sites, err := planSites(cfg, results)
		if err != nil {
			t.Fatalf("planSites: %v", err)
		}
		if len(sites) != 1 || sites[0].Name != "default" {
			t.Errorf("sites = %+v, want [default] only", sites)
		}
	})

	t.Run("outpost declarations still validated", func(t *testing.T) {
		cfg := testCfg(t)
		results := []SiteResult{
			{Slug: "op", CheckType: "outpost", Sites: map[string]string{"internal": "nope"}},
		}
		if _, err := planSites(cfg, results); err == nil {
			t.Error("planSites: expected error for outpost unknown level")
		}
	})
}

func TestLevelFor(t *testing.T) {
	t.Run("implicit full in default site", func(t *testing.T) {
		if got := levelFor("default", SiteResult{}); got != "full" {
			t.Errorf("levelFor(default, {}) = %q, want full", got)
		}
	})

	t.Run("declared level honored per site", func(t *testing.T) {
		r := SiteResult{Sites: map[string]string{"internal": "basic"}}
		if got := levelFor("internal", r); got != "basic" {
			t.Errorf("levelFor(internal) = %q, want basic", got)
		}
		if got := levelFor("default", r); got != "full" {
			t.Errorf("levelFor(default) = %q, want full", got)
		}
	})

	t.Run("different levels in different sites", func(t *testing.T) {
		r := SiteResult{Sites: map[string]string{"default": "basic", "internal": "full"}}
		if got := levelFor("default", r); got != "basic" {
			t.Errorf("levelFor(default) = %q, want basic", got)
		}
		if got := levelFor("internal", r); got != "full" {
			t.Errorf("levelFor(internal) = %q, want full", got)
		}
		if got := levelFor("zeta", r); got != "full" {
			t.Errorf("levelFor(zeta) = %q, want full (undeclared)", got)
		}
	})

	t.Run("empty declared level falls back to full", func(t *testing.T) {
		r := SiteResult{Sites: map[string]string{"internal": ""}}
		if got := levelFor("internal", r); got != "full" {
			t.Errorf("levelFor(internal) = %q, want full", got)
		}
	})
}

func TestSiteMembers(t *testing.T) {
	results := []SiteResult{
		{Slug: "http", CheckType: "http", OutpostSlug: "local", Sites: map[string]string{"internal": "basic"}},
		{Slug: "dns", CheckType: "dns", OutpostSlug: "local"},
		{Slug: "rem", CheckType: "http", OutpostSlug: "remote", Sites: map[string]string{"internal": "basic"}},
		{Slug: "local", CheckType: "outpost", OutpostSlug: "local"},
		{Slug: "remote", CheckType: "outpost", OutpostSlug: "remote"},
	}

	t.Run("default site includes all resources and derived outposts", func(t *testing.T) {
		resources, outposts := siteMembers("default", results)
		if len(resources) != 3 {
			t.Errorf("default resources = %d, want 3", len(resources))
		}
		if len(outposts) != 2 {
			t.Errorf("default outposts = %d, want 2", len(outposts))
		}
	})

	t.Run("extra site includes only members and their outposts", func(t *testing.T) {
		resources, outposts := siteMembers("internal", results)
		if len(resources) != 2 {
			t.Errorf("internal resources = %d, want 2", len(resources))
		}
		var slugs []string
		for _, r := range outposts {
			slugs = append(slugs, r.Slug)
		}
		want := []string{"local", "remote"}
		if !reflect.DeepEqual(slugs, want) {
			t.Errorf("internal outposts = %v, want %v", slugs, want)
		}
	})

	t.Run("outpost with no member resources is omitted", func(t *testing.T) {
		only := []SiteResult{
			{Slug: "http", CheckType: "http", OutpostSlug: "local"},
			{Slug: "remote", CheckType: "outpost", OutpostSlug: "remote"},
		}
		_, outposts := siteMembers("internal", only)
		if len(outposts) != 0 {
			t.Errorf("outposts = %+v, want none (no member resources)", outposts)
		}
	})
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
