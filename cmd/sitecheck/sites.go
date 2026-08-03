package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Site is a generated site directory. The implicit "default" site always exists; extra sites
// are discovered from resource site memberships.
type Site struct {
	Name string
}

// defaultSiteName is the implicit site every resource belongs to. Its membership cannot be
// removed, but a resource may change its level within it.
const defaultSiteName = "default"

// siteNameRe validates site names, which are used verbatim as output directory names.
var siteNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// planSites discovers the sites for this run from resource memberships (plus the implicit
// default) and validates every resource and outpost site declaration. Outpost declarations
// never create sites — an outpost appears only where it has member resources — but their site
// names and levels are validated like resources'.
func planSites(cfg *Config, results []SiteResult) ([]Site, error) {
	levels, err := knownLevels(cfg.TemplatesDir)
	if err != nil {
		return nil, err
	}

	siteSet := map[string]bool{defaultSiteName: true}
	for _, r := range results {
		for site, lvl := range r.Sites {
			if err := validateSiteDecl(site, lvl, levels); err != nil {
				return nil, fmt.Errorf("%s %q: %w", resultKind(r), r.Slug, err)
			}
			if r.CheckType != "outpost" {
				siteSet[site] = true
			}
		}
	}

	sites := make([]Site, 0, len(siteSet))
	sites = append(sites, Site{Name: defaultSiteName})
	extras := make([]string, 0, len(siteSet)-1)
	for name := range siteSet {
		if name != defaultSiteName {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		sites = append(sites, Site{Name: name})
	}
	return sites, nil
}

// knownLevels returns the available template levels: subdirectories of templatesDir that
// contain both card.html and resource.html. Adding a level is purely additive.
func knownLevels(templatesDir string) (map[string]bool, error) {
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("read templates dir %q: %w", templatesDir, err)
	}
	levels := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		card := filepath.Join(templatesDir, e.Name(), "card.html")
		res := filepath.Join(templatesDir, e.Name(), "resource.html")
		if fileExists(card) && fileExists(res) {
			levels[e.Name()] = true
		}
	}
	return levels, nil
}

// validateSiteDecl checks a single site name → level declaration.
func validateSiteDecl(site, lvl string, levels map[string]bool) error {
	if !siteNameRe.MatchString(site) {
		return fmt.Errorf("invalid site name %q (must match [A-Za-z0-9_-]+)", site)
	}
	if !levels[lvl] {
		return fmt.Errorf("unknown level %q for site %q (known levels: %s)", lvl, site, sortedLevels(levels))
	}
	return nil
}

// levelFor resolves the level at which a result renders within a site: the declared per-site
// level, or the implicit "full" when undeclared.
func levelFor(site string, r SiteResult) string {
	if lvl, ok := r.Sites[site]; ok && lvl != "" {
		return lvl
	}
	return "full"
}

// inSite reports whether a resource belongs to site. The default site is implicit and cannot be
// removed; extra sites require a declared membership.
func inSite(r SiteResult, site string) bool {
	if site == defaultSiteName {
		return true
	}
	_, ok := r.Sites[site]
	return ok
}

// siteMembers splits results into the resources and outpost results belonging to site. Resource
// membership is declared (default implicit); outpost membership is derived — an outpost appears
// in a site iff at least one of that site's resources runs on it.
func siteMembers(site string, results []SiteResult) (resources, outposts []SiteResult) {
	for _, r := range results {
		if r.CheckType == "outpost" {
			continue
		}
		if inSite(r, site) {
			resources = append(resources, r)
		}
	}
	outpostSlugs := make(map[string]bool, len(resources))
	for _, r := range resources {
		outpostSlugs[r.OutpostSlug] = true
	}
	for _, r := range results {
		if r.CheckType == "outpost" && outpostSlugs[r.Slug] {
			outposts = append(outposts, r)
		}
	}
	return resources, outposts
}

func resultKind(r SiteResult) string {
	if r.CheckType == "outpost" {
		return "outpost"
	}
	return "resource"
}

func sortedLevels(levels map[string]bool) string {
	names := make([]string, 0, len(levels))
	for lvl := range levels {
		names = append(names, lvl)
	}
	sort.Strings(names)
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%q", n)
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
