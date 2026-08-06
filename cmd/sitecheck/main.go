package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/joho/godotenv"

	"sitecheck/cmd/sitecheck/db"
	"sitecheck/core"
	"sitecheck/notify"

	_ "sitecheck/checktypes/dns"
	_ "sitecheck/checktypes/exec"
	_ "sitecheck/checktypes/http"
	_ "sitecheck/checktypes/outpost"
	_ "sitecheck/checktypes/ping"
	_ "sitecheck/checktypes/ssl"
	_ "sitecheck/checktypes/systemd"
	_ "sitecheck/checktypes/tcp"
)

func main() {
	// Load .env from working directory; not fatal if missing.
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: no .env file found, using defaults (%v)\n", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}
	// Build notification senders from env config.
	var senders []notify.Sender
	if cfg.NtfyServer != "" {
		senders = append(senders, &notify.NtfySender{URL: cfg.NtfyServer})
	}
	if cfg.TelegramToken != "" && cfg.TelegramChannel != "" {
		senders = append(senders, &notify.TelegramSender{Token: cfg.TelegramToken, Channel: cfg.TelegramChannel})
	}
	sender := &notify.Broadcast{Senders: senders}

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

	// Scan remote outpost definitions from outposts/ directory.
	remoteOutposts, err := scanOutposts(cfg.OutpostsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Outpost scan error: %v\n", err)
		os.Exit(1)
	}

	// Build local outpost definition. outposts/local.lua overrides name, skip, and notify_down; url and token are ignored
	// for local.
	localDef := OutpostDef{
		Slug:       "local",
		Name:       "Local",
		Token:      "local",
		Skip:       false,
		NotifyDown: true,
	}
	if localOverride, err := loadLocalOverride(cfg.OutpostsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Local outpost override error: %v\n", err)
	} else if localOverride != nil {
		if localOverride.Name != "" {
			localDef.Name = localOverride.Name
		}
		localDef.Skip = localOverride.Skip
		localDef.NotifyDown = localOverride.NotifyDown
		localDef.Sites = localOverride.Sites
	}

	// Combine: local always first, then remotes.
	allOutposts := append([]OutpostDef{localDef}, remoteOutposts...)
	// Collect results from all outposts via the pool.
	var siteResults []SiteResult
	// Unrecognized wire versions are warned about once per distinct value, so a single bad
	// outpost cannot spam the log.
	seenVersions := make(map[string]bool)

	for pr := range runOutpostPool(allOutposts, cfg) {
		if pr.Err != nil {
			siteResults = append(siteResults, handleOutpostDown(database, sender, allOutposts, pr)...)
			continue
		}
		siteResults = append(siteResults, handleWireResult(database, sender, allOutposts, pr, seenVersions))
	}

	// Query history for the charts: the 30-day chart needs 30 days of data; retention
	// governs how much is kept, not how much is queried.
	since := time.Now().Add(-time.Duration(chartWindow30d) * time.Hour)

	for i := range siteResults {
		r := &siteResults[i]
		if r.CheckType == "" {
			continue
		}
		r.History = queryTypedHistory(database, r.Slug, r.OutpostSlug, r.CheckType, since)
	}

	// Sort alphabetically by slug.
	sort.Slice(siteResults, func(i, j int) bool {
		return siteResults[i].Slug < siteResults[j].Slug
	})

	// Purge old checks.
	if err := database.PurgeOld(cfg.RetentionDays); err != nil {
		fmt.Fprintf(os.Stderr, "Purge error: %v\n", err)
	}

	// Generate static site.
	if err := Generate(cfg, siteResults); err != nil {
		fmt.Fprintf(os.Stderr, "Sitegen error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done.")
}

// handleOutpostDown processes a failed outpost: notifies on the up→down
// transition, synthesizes UNKNOWN (or FAIL for the outpost itself) results
// from prior DB history, and returns them for display.
func handleOutpostDown(database *db.DB, sender notify.Sender, outposts []OutpostDef, pr PoolResult) []SiteResult {
	fmt.Fprintf(os.Stderr, "Outpost %q failed: %v\n", pr.OutpostSlug, pr.Err)

	// Notify on up→down transition only.
	outpost := findOutpost(outposts, pr.OutpostSlug)
	if outpost != nil && outpost.NotifyDown {
		prevPass, hasPrev, _ := database.LastPass(outpost.Slug, outpost.Slug, "outpost")
		if hasPrev && prevPass == core.PASS {
			title := fmt.Sprintf("SiteCheck: Outpost %s DOWN", outpost.Name)
			msg := fmt.Sprintf("Outpost %s is unreachable: %v", outpost.Name, pr.Err)
			sender.Send(notify.Message{Title: title, Body: msg})
		}
	}

	// Insert UNKNOWN rows for resources with prior history from this outpost.
	slugTypes, dbErr := database.DistinctSlugsByOutpost(pr.OutpostSlug)
	if dbErr != nil {
		fmt.Fprintf(os.Stderr, "  distinct slugs error: %v\n", dbErr)
		return nil
	}
	if len(slugTypes) == 0 {
		fmt.Fprintf(os.Stderr, "  No prior history for outpost %q — skipping UNKNOWN rows.\n", pr.OutpostSlug)
		return nil
	}

	var results []SiteResult
	for _, st := range slugTypes {
		pass := core.UNKNOWN
		label := "UNKNOWN"
		if st.Type == "outpost" {
			pass = core.FAIL
			label = "FAIL"
		}
		wr := unknownWireResult(st.Slug, st.Type, pr.OutpostSlug, pass, pr.Err)
		if err := insertWireResult(database, wr); err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s DB ERROR (unknown): %v\n", st.Slug, err)
			continue
		}
		fmt.Printf("  %-20s %s outpost down\n", st.Slug, label)
		errMsg := wr.Error
		if st.Type != "outpost" && outpost != nil {
			errMsg = fmt.Sprintf("Outpost %s is down", outpost.Name)
		}
		// Resources keep their persisted site membership (shown as unknown); the
		// outpost's own result gets the levels from its definition file.
		var sites map[string]string
		if st.Type == "outpost" {
			if outpost != nil {
				sites = outpost.Sites
			}
		} else if m, ok := database.ResourceMeta(st.Slug, pr.OutpostSlug); ok {
			sites = m
		}
		results = append(results, SiteResult{
			Slug:        st.Slug,
			Name:        st.Slug,
			CheckType:   st.Type,
			Pass:        pass,
			Err:         errMsg,
			OutpostSlug: pr.OutpostSlug,
			OutpostName: pr.OutpostName,
			Sites:       sites,
		})
	}
	return results
}

// handleWireResult processes one result from a live outpost: resolves the
// sentinel check type, persists site membership, inserts the row, notifies on
// status changes, and returns the SiteResult for display.
func handleWireResult(database *db.DB, sender notify.Sender, outposts []OutpostDef, pr PoolResult, seenVersions map[string]bool) SiteResult {
	wr := pr.WireResult
	compositeSlug := wr.OutpostSlug + "-" + wr.Slug

	// Wire format version check: an absent field means the old format (version 1, known).
	// An unrecognized version is still parsed best-effort — 1.1 is backwards compatible by
	// construction — so we only warn, once per distinct version value.
	if !core.IsKnownWireVersion(wr.Version) {
		if !seenVersions[wr.Version] {
			seenVersions[wr.Version] = true
			fmt.Fprintf(os.Stderr, "Warning: unrecognized wire format version %q from outpost %q — parsing best-effort\n",
				wr.Version, pr.OutpostSlug)
		}
	}

	// Resolve sentinel check type from DB history.
	if wr.CheckType == core.CheckTypeLuaError {
		wr.Pass = core.UNKNOWN
		if typ, ok := database.LookupCheckType(wr.Slug, wr.OutpostSlug); ok {
			wr.CheckType = typ
		} else {
			wr.CheckType = "http"
		}
	}

	// Persist site membership for real resource results (not outposts — their levels come
	// from the definition file each run). This survives the downed-outpost case, where no
	// fresh meta() data arrives and the core must synthesize results from the DB.
	if wr.CheckType != "outpost" {
		if err := database.UpsertResourceMeta(wr.Slug, wr.OutpostSlug, wr.Sites); err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s DB ERROR (meta): %v\n", compositeSlug, err)
		}
	}

	prevPass, hasPrev, _ := database.LastPass(wr.Slug, wr.OutpostSlug, wr.CheckType)

	currentPass := wr.Pass
	if wr.Error != "" && currentPass != core.UNKNOWN {
		currentPass = core.FAIL
	}

	if err := insertWireResult(database, *wr); err != nil {
		fmt.Fprintf(os.Stderr, "  %-20s DB ERROR: %v\n", compositeSlug, err)
	} else {
		fmt.Printf("  %-20s type=%-6s %s %s\n",
			compositeSlug, wr.CheckType, passName(currentPass),
			time.Duration(wr.ElapsedMS)*time.Millisecond)
		if wr.CheckType != "outpost" {
			reason := wr.FailReason
			if reason == "" {
				reason = wr.Error
			}
			notifyStatusChange(sender, statusChange{
				Slug:           compositeSlug,
				Name:           wr.Name,
				FailReason:     reason,
				CurrentPass:    currentPass,
				PrevPass:       prevPass,
				HasPrev:        hasPrev,
				NotifyPass:     wr.NotifyPass,
				NotifyDegraded: wr.NotifyDegraded,
				NotifyFail:     wr.NotifyFail,
			})
		} else {
			// Outpost health check: notify on down→up transition.
			outpost := findOutpost(outposts, pr.OutpostSlug)
			if outpost != nil && outpost.NotifyDown && hasPrev && prevPass == core.FAIL {
				title := fmt.Sprintf("SiteCheck: Outpost %s UP", outpost.Name)
				msg := fmt.Sprintf("Outpost %s is reachable again", outpost.Name)
				sender.Send(notify.Message{Title: title, Body: msg})
			}
		}
	}

	return SiteResult{
		Slug:        wr.Slug,
		Name:        wr.Name,
		Desc:        wr.Desc,
		CheckType:   wr.CheckType,
		Pass:        currentPass,
		FailReason:  wr.FailReason,
		ResponseMS:  wr.ResponseMS,
		Err:         wr.Error,
		OutpostSlug: wr.OutpostSlug,
		OutpostName: pr.OutpostName,
		Sites:       wr.Sites,
	}
}

// queryTypedHistory returns the full typed DB check history for a slug+type.
func queryTypedHistory(database *db.DB, slug, outpostSlug, checkType string, since time.Time) interface{} {
	p, ok := core.ByName(checkType)
	if !ok {
		return nil
	}
	h, err := p.QuerySince(database.DB, slug, outpostSlug, since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
		return nil
	}
	return h
}

// insertWireResult deserializes the typed Data from a WireResult and inserts it into the appropriate DB check table.
// wr.OutpostSlug must be set by the caller.
func insertWireResult(database *db.DB, wr core.WireResult) error {
	if wr.Error != "" {
		return insertErrorResult(database, wr)
	}
	p, ok := core.ByName(wr.CheckType)
	if !ok {
		return fmt.Errorf("unknown check type %q", wr.CheckType)
	}
	return p.Insert(database.DB, wr.Slug, wr.OutpostSlug, wr.ElapsedMS, wr.Data)
}

// insertErrorResult inserts a minimal row for a check that produced an error without typed data.
func insertErrorResult(database *db.DB, wr core.WireResult) error {
	p, ok := core.ByName(wr.CheckType)
	if !ok {
		return fmt.Errorf("unknown check type %q for error insert", wr.CheckType)
	}
	return p.InsertError(database.DB, wr.Slug, wr.OutpostSlug, wr.ElapsedMS, wr.Pass, wr.Error)
}

// loadLocalOverride reads local.lua from the configured outposts dir if it exists and returns the overrides for the
// implicit local outpost. URL and token are ignored.
func loadLocalOverride(outpostsDir string) (*OutpostDef, error) {
	path := filepath.Join(outpostsDir, "local.lua")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	def, err := loadOutpostDef("local", path)
	if err != nil {
		return nil, err
	}
	return &def, nil
}

// findOutpost returns the OutpostDef with the given slug, or nil.
func findOutpost(outposts []OutpostDef, slug string) *OutpostDef {
	for i := range outposts {
		if outposts[i].Slug == slug {
			return &outposts[i]
		}
	}
	return nil
}

// unknownWireResult creates a synthetic WireResult representing an outpost-down state. It has no typed data — only the
// error field is populated.
func unknownWireResult(slug, checkType, outpostSlug string, pass int, err error) core.WireResult {
	return core.WireResult{
		Slug:        slug,
		Name:        slug,
		CheckType:   checkType,
		OutpostSlug: outpostSlug,
		Pass:        pass,
		Error:       fmt.Sprintf("outpost unreachable: %v", err),
		Version:     core.WireVersion,
	}
}
