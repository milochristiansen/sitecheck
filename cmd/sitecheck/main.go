package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/joho/godotenv"

	"sitecheck/protocol"
	"sitecheck/cmd/sitecheck/db"
)

// UNKNOWN is the pass value injected by the core when an outpost is unreachable. It is never returned by a resource
// script and is not exported to lmods.
const UNKNOWN = -1

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
	remoteOutposts, err := scanOutposts()
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
	if localOverride, err := loadLocalOverride(); err != nil {
		fmt.Fprintf(os.Stderr, "Local outpost override error: %v\n", err)
	} else if localOverride != nil {
		if localOverride.Name != "" {
			localDef.Name = localOverride.Name
		}
		localDef.Skip = localOverride.Skip
		localDef.NotifyDown = localOverride.NotifyDown
	}

	// Combine: local always first, then remotes.
	allOutposts := append([]OutpostDef{localDef}, remoteOutposts...)

	// Collect results from all outposts via the pool.
	var siteResults []SiteResult

	for pr := range runOutpostPool(allOutposts, cfg) {
		if pr.Err != nil {
			// Outpost failed as a whole.
			fmt.Fprintf(os.Stderr, "Outpost %q failed: %v\n", pr.OutpostSlug, pr.Err)

			// Notify if configured.
			outpost := findOutpost(allOutposts, pr.OutpostSlug)
			if outpost != nil && outpost.NotifyDown {
				title := fmt.Sprintf("SiteCheck: Outpost %s DOWN", outpost.Name)
				msg := fmt.Sprintf("Outpost %s is unreachable: %v", outpost.Name, pr.Err)
				notify(cfg.NtfyServer, title, msg)
			}

			// Insert UNKNOWN rows for resources with prior history from this outpost.
			slugTypes, dbErr := database.DistinctSlugsByOutpost(pr.OutpostSlug)
			if dbErr != nil {
				fmt.Fprintf(os.Stderr, "  distinct slugs error: %v\n", dbErr)
				continue
			}
			if len(slugTypes) == 0 {
				fmt.Fprintf(os.Stderr, "  No prior history for outpost %q — skipping UNKNOWN rows.\n", pr.OutpostSlug)
				continue
			}
			for _, st := range slugTypes {
				pass := UNKNOWN
				label := "UNKNOWN"
				if st.Type == "outpost" {
					pass = protocol.FAIL
					label = "FAIL"
				}
				wr := unknownWireResult(st.Slug, st.Type, pr.OutpostSlug, pass, pr.Err)
				if err := insertWireResult(database, wr); err != nil {
					fmt.Fprintf(os.Stderr, "  %-20s DB ERROR (unknown): %v\n", st.Slug, err)
				} else {
					fmt.Printf("  %-20s %s outpost down\n", st.Slug, label)
					siteResults = append(siteResults, SiteResult{
						Slug:        st.Slug,
						Name:        st.Slug,
						CheckType:   st.Type,
						Pass:        pass,
						Err:         wr.Error,
						OutpostSlug: pr.OutpostSlug,
					})
				}
			}
			continue
		}

		// Normal result from a working outpost.
		wr := pr.WireResult
		compositeSlug := wr.OutpostSlug + "-" + wr.Slug

		prevPass, hasPrev, _ := database.LastPass(wr.Slug, wr.OutpostSlug, wr.CheckType)

		currentPass := wr.Pass
		if wr.Error != "" {
			currentPass = protocol.FAIL
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
				notifyStatusChange(cfg.NtfyServer, compositeSlug, wr.Name, reason,
					currentPass, prevPass, hasPrev,
					wr.NotifyPass, wr.NotifyDegraded, wr.NotifyFail)
			}
		}

		siteResults = append(siteResults, SiteResult{
			Slug:        wr.Slug,
			Name:        wr.Name,
			Desc:        wr.Desc,
			CheckType:   wr.CheckType,
			Pass:        currentPass,
			FailReason:  wr.FailReason,
			ResponseMS:  wr.ResponseMS,
			Err:         wr.Error,
			OutpostSlug: wr.OutpostSlug,
		})
	}

	// Query history for the largest graph window.
	maxWindow := 24
	for _, w := range cfg.GraphWindows {
		if w > maxWindow {
			maxWindow = w
		}
	}
	since := time.Now().Add(-time.Duration(maxWindow) * time.Hour)

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

// queryTypedHistory returns the full typed DB check history for a slug+type.
func queryTypedHistory(database *db.DB, slug, outpostSlug, checkType string, since time.Time) interface{} {
	switch checkType {
	case "http":
		h, err := db.HTTPChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.HTTPCheck(nil)
		}
		return h
	case "ping":
		h, err := db.PingChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.PingCheck(nil)
		}
		return h
	case "tcp":
		h, err := db.TCPChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.TCPCheck(nil)
		}
		return h
	case "dns":
		h, err := db.DNSChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.DNSCheck(nil)
		}
		return h
	case "ssl":
		h, err := db.SSLChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.SSLCheck(nil)
		}
		return h
	case "systemd":
		h, err := db.SystemdChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.SystemdCheck(nil)
		}
		return h
	case "outpost":
		h, err := db.OutpostChecksBySlugSince(database, slug, outpostSlug, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %-20s history query error: %v\n", slug, err)
			return []db.OutpostCheck(nil)
		}
		return h
	default:
		return nil
	}
}

// insertWireResult deserializes the typed Data from a WireResult and inserts it into the appropriate DB check table.
// wr.OutpostSlug must be set by the caller.
func insertWireResult(database *db.DB, wr protocol.WireResult) error {
	slug := wr.Slug
	if wr.Error != "" {
		return insertErrorResult(database, slug, wr)
	}

	switch wr.CheckType {
	case "http":
		var r protocol.HTTPResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal http data: %w", err)
		}
		return insertHTTP(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	case "ping":
		var r protocol.PingResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal ping data: %w", err)
		}
		return insertPing(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	case "tcp":
		var r protocol.TCPResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal tcp data: %w", err)
		}
		return insertTCP(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	case "dns":
		var r protocol.DNSResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal dns data: %w", err)
		}
		return insertDNS(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	case "ssl":
		var r protocol.SSLResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal ssl data: %w", err)
		}
		return insertSSL(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	case "systemd":
		var r protocol.SystemdResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal systemd data: %w", err)
		}
		return insertSystemd(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	case "outpost":
		var r protocol.OutpostResult
		if err := json.Unmarshal(wr.Data, &r); err != nil {
			return fmt.Errorf("unmarshal outpost data: %w", err)
		}
		return insertOutpost(database, slug, wr.OutpostSlug, wr.ElapsedMS, &r)
	default:
		return fmt.Errorf("unknown check type %q", wr.CheckType)
	}
}

// insertErrorResult inserts a minimal row for a check that produced an error without typed data.
func insertErrorResult(database *db.DB, slug string, wr protocol.WireResult) error {
	switch wr.CheckType {
	case "http":
		_, e := db.InsertHTTPCheck(database, db.HTTPCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error, URL: "(error)",
		})
		return e
	case "ping":
		_, e := db.InsertPingCheck(database, db.PingCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error, Host: "(error)",
		})
		return e
	case "tcp":
		_, e := db.InsertTCPCheck(database, db.TCPCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error, Host: "(error)", Port: 0,
		})
		return e
	case "dns":
		_, e := db.InsertDNSCheck(database, db.DNSCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error, Host: "(error)",
		})
		return e
	case "ssl":
		_, e := db.InsertSSLCheck(database, db.SSLCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error, Host: "(error)", Port: 0,
		})
		return e
	case "systemd":
		_, e := db.InsertSystemdCheck(database, db.SystemdCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error, ServiceName: "(error)",
		})
		return e
	case "outpost":
		_, e := db.InsertOutpostCheck(database, db.OutpostCheck{
			Slug: slug, OutpostSlug: wr.OutpostSlug, DurationMS: wr.ElapsedMS,
			Pass: wr.Pass, Error: wr.Error,
		})
		return e
	default:
		return fmt.Errorf("unknown check type %q for error insert", wr.CheckType)
	}
}

// --- Typed insert helpers ---

func insertHTTP(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.HTTPResult) error {
	c := db.HTTPCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		StatusCode:     r.StatusCode,
		URL:            r.URL,
		BodySize:       r.BodySize,
		TLSVersion:     r.TLSVersion,
		RemoteIP:       r.RemoteIP,
		RedirectCount:  r.RedirectCount,
		Error:          r.Error,
	}
	if r.Body != "" {
		c.Body = &r.Body
	}
	_, err := db.InsertHTTPCheck(database, c)
	return err
}

func insertPing(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.PingResult) error {
	c := db.PingCheck{
		Slug:            slug,
		OutpostSlug:     outpostSlug,
		DurationMS:      elapsedMS,
		Pass:            r.Pass,
		ResponseTimeMS:  r.ResponseTimeMS,
		PacketsSent:     r.PacketsSent,
		PacketsReceived: r.PacketsReceived,
		PacketLossPct:   r.PacketLossPct,
		MinMS:           r.MinMS,
		MaxMS:           r.MaxMS,
		Host:            r.Host,
		Error:           r.Error,
	}
	_, err := db.InsertPingCheck(database, c)
	return err
}

func insertTCP(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.TCPResult) error {
	c := db.TCPCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		Host:           r.Host,
		Port:           r.Port,
		RemoteIP:       r.RemoteIP,
		Error:          r.Error,
	}
	_, err := db.InsertTCPCheck(database, c)
	return err
}

func insertDNS(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.DNSResult) error {
	ipsJSON := "["
	for i, ip := range r.IPs {
		if i > 0 {
			ipsJSON += ","
		}
		ipsJSON += `"` + ip + `"`
	}
	ipsJSON += "]"

	c := db.DNSCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		Host:           r.Host,
		IPs:            ipsJSON,
		Error:          r.Error,
	}
	_, err := db.InsertDNSCheck(database, c)
	return err
}

func insertSSL(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.SSLResult) error {
	c := db.SSLCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		Host:           r.Host,
		Port:           r.Port,
		Issuer:         r.Issuer,
		Subject:        r.Subject,
		NotBefore:      r.NotBefore,
		NotAfter:       r.NotAfter,
		DaysRemaining:  r.DaysRemaining,
		Error:          r.Error,
	}
	_, err := db.InsertSSLCheck(database, c)
	return err
}

func insertSystemd(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.SystemdResult) error {
	c := db.SystemdCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		ServiceName:    r.ServiceName,
		ActiveState:    r.ActiveState,
		SubState:       r.SubState,
		LoadState:      r.LoadState,
		MainPID:        r.MainPID,
		Error:          r.Error,
	}
	_, err := db.InsertSystemdCheck(database, c)
	return err
}

func insertOutpost(database *db.DB, slug, outpostSlug string, elapsedMS int64, r *protocol.OutpostResult) error {
	c := db.OutpostCheck{
		Slug:           slug,
		OutpostSlug:    outpostSlug,
		DurationMS:     elapsedMS,
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		CheckCount:     r.CheckCount,
		FailCount:      r.FailCount,
		Error:          r.Error,
	}
	_, err := db.InsertOutpostCheck(database, c)
	return err
}

// loadLocalOverride reads outposts/local.lua if it exists and returns the overrides for the implicit local outpost. URL
// and token are ignored.
func loadLocalOverride() (*OutpostDef, error) {
	path := filepath.Join("outposts", "local.lua")
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
func unknownWireResult(slug, checkType, outpostSlug string, pass int, err error) protocol.WireResult {
	return protocol.WireResult{
		Slug:        slug,
		Name:        slug,
		CheckType:   checkType,
		OutpostSlug: outpostSlug,
		Pass:        pass,
		Error:       fmt.Sprintf("outpost unreachable: %v", err),
	}
}
