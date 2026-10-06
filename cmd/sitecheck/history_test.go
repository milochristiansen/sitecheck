package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"sitecheck/checktypes/exec"
	"sitecheck/checktypes/http"
	"sitecheck/cmd/sitecheck/db"
	"sitecheck/core"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func insertHTTP(t *testing.T, database *db.DB, body string, respMS float64) int64 {
	t.Helper()
	p, ok := core.ByName("http")
	if !ok {
		t.Fatal("http plugin not registered")
	}
	data, _ := json.Marshal(map[string]any{
		"Pass":           core.PASS,
		"Body":           body,
		"BodySize":       len(body),
		"ResponseTimeMS": respMS,
		"URL":            "https://example.com",
		"StatusCode":     200,
	})
	if err := p.Insert(database.DB, "site", "local", 5, data); err != nil {
		t.Fatalf("insert http: %v", err)
	}
	var id int64
	if err := database.QueryRow("SELECT id FROM checks_http ORDER BY id DESC LIMIT 1").Scan(&id); err != nil {
		t.Fatalf("last http id: %v", err)
	}
	return id
}

func insertExec(t *testing.T, database *db.DB, stdout, stderr, combined string) string {
	t.Helper()
	p, ok := core.ByName("exec")
	if !ok {
		t.Fatal("exec plugin not registered")
	}
	data, _ := json.Marshal(map[string]any{
		"Pass":           core.PASS,
		"Command":        "echo hi",
		"ExitCode":       0,
		"Stdout":         stdout,
		"Stderr":         stderr,
		"Combined":       combined,
		"ResponseTimeMS": 1.0,
	})
	if err := p.Insert(database.DB, "site", "local", 5, data); err != nil {
		t.Fatalf("insert exec: %v", err)
	}
	var id string
	if err := database.QueryRow("SELECT id FROM checks_exec ORDER BY rowid DESC LIMIT 1").Scan(&id); err != nil {
		t.Fatalf("last exec id: %v", err)
	}
	return id
}

// TestHistorySourceHTTP exercises the narrow/light/full read split against real
// SQLite: points carry no body, light rows carry the body hash, and only LoadFull
// returns the body.
func TestHistorySourceHTTP(t *testing.T) {
	database := newTestDB(t)
	src := dbSource{db: database}

	bodies := []string{"alpha body", "alpha body", "beta body"}
	ids := make([]int64, len(bodies))
	for i, b := range bodies {
		ids[i] = insertHTTP(t, database, b, float64(10+i))
	}
	t0 := time.Now().UTC().Add(-30 * time.Minute)
	for i, id := range ids {
		ts := t0.Add(time.Duration(i) * time.Minute).Format("2006-01-02 15:04:05")
		if _, err := database.Exec("UPDATE checks_http SET timestamp = ? WHERE id = ?", ts, id); err != nil {
			t.Fatalf("set timestamp: %v", err)
		}
	}

	pts, err := src.Points("http", "site", "local", time.Time{}, 0)
	if err != nil {
		t.Fatalf("Points: %v", err)
	}
	if len(pts) != 3 {
		t.Fatalf("Points = %d, want 3", len(pts))
	}
	if pts[0].Resp != 10 || pts[2].Resp != 12 {
		t.Errorf("Points response times = %v, want 10..12", []float64{pts[0].Resp, pts[1].Resp, pts[2].Resp})
	}

	last, err := src.Points("http", "site", "local", time.Time{}, 2)
	if err != nil {
		t.Fatalf("Points(limit): %v", err)
	}
	if len(last) != 2 || last[0].Resp != 11 || last[1].Resp != 12 {
		t.Errorf("Points(limit=2) = %+v, want resp 11,12", last)
	}

	var rows []http.HTTPCheck
	var idsOut []string
	err = src.EachRecentLight("http", "site", "local", time.Time{}, func(id string, row interface{}) error {
		rows = append(rows, row.(http.HTTPCheck))
		idsOut = append(idsOut, id)
		return nil
	})
	if err != nil {
		t.Fatalf("EachRecentLight: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("light rows = %d, want 3", len(rows))
	}
	// Newest-first.
	if rows[0].ResponseTimeMS != 12 || rows[2].ResponseTimeMS != 10 {
		t.Errorf("light order resp = %v, want 12,11,10", []float64{rows[0].ResponseTimeMS, rows[1].ResponseTimeMS, rows[2].ResponseTimeMS})
	}
	for i, r := range rows {
		if r.Body != nil {
			t.Errorf("light row %d has Body, want omitted", i)
		}
		if r.BodyHash == nil {
			t.Errorf("light row %d missing BodyHash", i)
		}
	}
	// rows[0] is the newest (beta body); rows[1] and rows[2] are the two alpha bodies.
	if core.Similar(rows[0], rows[1]) {
		t.Error("light rows with different body hashes must not be Similar")
	}
	if !core.Similar(rows[1], rows[2]) {
		t.Error("light rows with identical body hashes must be Similar")
	}

	full, err := src.LoadFull("http", idsOut[0])
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	fullRow := full.(http.HTTPCheck)
	if fullRow.Body == nil || *fullRow.Body != "beta body" {
		t.Errorf("LoadFull Body = %v, want %q", fullRow.Body, "beta body")
	}
	if fullRow.BodyHash == nil {
		t.Error("LoadFull missing BodyHash")
	}
}

// TestHistorySourceExec mirrors the HTTP test for the three exec output fields.
func TestHistorySourceExec(t *testing.T) {
	database := newTestDB(t)
	src := dbSource{db: database}

	large := strings.Repeat("z", 5000)
	idA := insertExec(t, database, large, "", large)
	idB := insertExec(t, database, large, "", large)
	idC := insertExec(t, database, "different output", "", "different output")
	// Spread timestamps using rowid-independent updates.
	now := time.Now().UTC()
	for i, id := range []string{idC, idB, idA} {
		ts := now.Add(-time.Duration(i) * time.Minute).Format("2006-01-02 15:04:05")
		if _, err := database.Exec("UPDATE checks_exec SET timestamp = ? WHERE id = ?", ts, id); err != nil {
			t.Fatalf("set timestamp: %v", err)
		}
	}

	var rows []exec.ExecCheck
	var idsOut []string
	err := src.EachRecentLight("exec", "site", "local", time.Time{}, func(id string, row interface{}) error {
		rows = append(rows, row.(exec.ExecCheck))
		idsOut = append(idsOut, id)
		return nil
	})
	if err != nil {
		t.Fatalf("EachRecentLight: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("light rows = %d, want 3", len(rows))
	}
	if rows[0].Stdout != "" || rows[0].Combined != "" {
		t.Error("light exec rows must omit stdout/combined")
	}
	if rows[0].StdoutHash == nil || rows[0].CombinedHash == nil || rows[0].StderrHash == nil {
		t.Error("light exec rows must carry all output hashes")
	}
	// rows[0] is the different output; rows[1] and rows[2] share the large output.
	if core.Similar(rows[0], rows[1]) {
		t.Error("light exec rows with different output must not be Similar")
	}
	if !core.Similar(rows[1], rows[2]) {
		t.Error("light exec rows with identical output must be Similar")
	}

	full, err := src.LoadFull("exec", idsOut[0])
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if full.(exec.ExecCheck).Stdout != "different output" {
		t.Errorf("LoadFull stdout = %q, want %q", full.(exec.ExecCheck).Stdout, "different output")
	}
}

// TestNeedsHydration pins which types require a full-row fetch.
func TestNeedsHydration(t *testing.T) {
	for _, tc := range []struct {
		typ  string
		want bool
	}{
		{"http", true},
		{"exec", true},
		{"dns", false},
		{"tcp", false},
		{"ping", false},
		{"ssl", false},
		{"systemd", false},
		{"outpost", false},
	} {
		p, ok := core.ByName(tc.typ)
		if !ok {
			t.Fatalf("plugin %q not registered", tc.typ)
		}
		if got := p.NeedsHydration(); got != tc.want {
			t.Errorf("%s.NeedsHydration() = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

// TestGenerateEndToEnd drives Generate with the real templates and a real DB to
// prove the lazy pipeline still writes hydrated check bodies and elision markers.
func TestGenerateEndToEnd(t *testing.T) {
	database := newTestDB(t)
	src := dbSource{db: database}

	p, ok := core.ByName("http")
	if !ok {
		t.Fatal("http plugin not registered")
	}
	base := time.Now().UTC().Add(-time.Hour)
	insert := func(body string, pass int, respMS float64, at time.Time) {
		t.Helper()
		data, _ := json.Marshal(map[string]any{
			"Pass":           pass,
			"Body":           body,
			"BodySize":       len(body),
			"ResponseTimeMS": respMS,
			"URL":            "https://example.com",
			"StatusCode":     200,
		})
		if err := p.Insert(database.DB, "site", "local", 5, data); err != nil {
			t.Fatalf("insert: %v", err)
		}
		var id int64
		if err := database.QueryRow("SELECT id FROM checks_http ORDER BY id DESC LIMIT 1").Scan(&id); err != nil {
			t.Fatalf("id: %v", err)
		}
		if _, err := database.Exec("UPDATE checks_http SET timestamp = ? WHERE id = ?", at.Format("2006-01-02 15:04:05"), id); err != nil {
			t.Fatalf("timestamp: %v", err)
		}
	}
	// Three identical PASS rows (should elide) plus a newer FAIL with unique body.
	insert("same body", core.PASS, 10, base.Add(-5*time.Minute))
	insert("same body", core.PASS, 11, base.Add(-4*time.Minute))
	insert("same body", core.PASS, 12, base.Add(-3*time.Minute))
	insert("unique body", core.FAIL, 0, base.Add(-1*time.Minute))

	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	cfg := &Config{
		SiteTitle:    "Test Site",
		TemplatesDir: filepath.Join(root, "templates"),
		StaticDir:    filepath.Join(root, "static"),
		OutputDir:    t.TempDir(),
	}
	results := []SiteResult{{
		Slug:        "site",
		Name:        "Site Check",
		CheckType:   "http",
		OutpostSlug: "local",
		OutpostName: "Local",
	}}
	if err := Generate(cfg, results, src); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	outPath := filepath.Join(cfg.OutputDir, "default", "resources", "local-site.html")
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read %s: %v", outPath, err)
	}
	out := string(raw)
	if !strings.Contains(out, "unique body") {
		t.Error("latest check body not hydrated into output")
	}
	if !strings.Contains(out, "same body") {
		t.Error("representative check body not hydrated into output")
	}
	if !strings.Contains(out, "(2 similar PASS checks elided)") {
		t.Error("expected elision marker for the three identical PASS rows")
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "default", "index.html")); err != nil {
		t.Errorf("index.html not written: %v", err)
	}
}

// TestGenerateTestingGround is a broad smoke test: it generates the whole site
// from a copy of the checked-in testing-ground database, exercising every check
// plugin's narrow/light/full reads. Skipped when the fixture is absent.
func TestGenerateTestingGround(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	srcDB := filepath.Join(root, "testing_ground", "work", "data", "sitecheck.db")
	if _, err := os.Stat(srcDB); err != nil {
		t.Skipf("testing-ground db not present: %v", err)
	}

	dir := t.TempDir()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, err := os.ReadFile(srcDB + suffix)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "sitecheck.db"+suffix), raw, 0o644); err != nil {
			t.Fatalf("copy db: %v", err)
		}
	}

	database, err := db.Open(filepath.Join(dir, "sitecheck.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var results []SiteResult
	seen := map[string]bool{}
	for _, p := range core.All() {
		rows, err := database.Query("SELECT DISTINCT slug, outpost_slug FROM " + p.TableName())
		if err != nil {
			t.Fatalf("distinct %s: %v", p.TableName(), err)
		}
		for rows.Next() {
			var slug, outpost string
			if err := rows.Scan(&slug, &outpost); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", p.TableName(), err)
			}
			key := p.TypeName() + "|" + slug + "|" + outpost
			if seen[key] {
				continue
			}
			seen[key] = true
			r := SiteResult{Slug: slug, Name: slug, CheckType: p.TypeName(), OutpostSlug: outpost, OutpostName: outpost}
			if p.TypeName() != "outpost" {
				if m, ok := database.ResourceMeta(slug, outpost); ok {
					r.Sites = m
				}
			}
			results = append(results, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("rows %s: %v", p.TableName(), err)
		}
	}
	if len(results) == 0 {
		t.Skip("testing-ground db has no check rows")
	}

	cfg := &Config{
		SiteTitle:    "Testing Ground",
		TemplatesDir: filepath.Join(root, "templates"),
		StaticDir:    filepath.Join(root, "static"),
		OutputDir:    filepath.Join(t.TempDir(), "output"),
	}
	if err := Generate(cfg, results, dbSource{db: database}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "default", "index.html")); err != nil {
		t.Errorf("default index not written: %v", err)
	}
}

// TestHashBackfillOnUpgrade simulates upgrading a database that predates the hash
// columns: the table is created without body_hash, Migrate adds the column via
// ALTER, and the one-time backfill fills in the hash.
func TestHashBackfillOnUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE checks_http (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL,
		outpost_slug TEXT NOT NULL DEFAULT '',
		timestamp TEXT,
		duration_ms INTEGER,
		pass INTEGER NOT NULL,
		response_time_ms REAL,
		status_code INTEGER,
		url TEXT NOT NULL,
		body_size INTEGER,
		body TEXT,
		tls_version TEXT,
		remote_ip TEXT,
		redirect_count INTEGER,
		error TEXT
	)`); err != nil {
		t.Fatalf("create old table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO checks_http (slug, outpost_slug, pass, url, body) VALUES ('site', 'local', 2, 'https://x', 'old body')`); err != nil {
		t.Fatalf("insert old row: %v", err)
	}
	raw.Close()

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate (upgrade): %v", err)
	}

	var hash sql.NullString
	if err := database.QueryRow(`SELECT body_hash FROM checks_http WHERE slug = 'site'`).Scan(&hash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if !hash.Valid {
		t.Fatal("body_hash not backfilled")
	}
	if hash.String != core.ContentHash("old body") {
		t.Errorf("body_hash = %q, want %q", hash.String, core.ContentHash("old body"))
	}

	// A second migrate must be a cheap no-op.
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate (idempotent): %v", err)
	}
}

// countingSource wraps a historySource and counts the expensive full-row reads.
type countingSource struct {
	historySource
	loads int
}

func (c *countingSource) LoadFull(checkType, id string) (interface{}, error) {
	c.loads++
	return c.historySource.LoadFull(checkType, id)
}

func insertHTTPAt(t *testing.T, database *db.DB, body string, pass int, at time.Time) {
	t.Helper()
	p, _ := core.ByName("http")
	data, _ := json.Marshal(map[string]any{
		"Pass":           pass,
		"Body":           body,
		"BodySize":       len(body),
		"ResponseTimeMS": 1.0,
		"URL":            "https://example.com",
		"StatusCode":     200,
	})
	if err := p.Insert(database.DB, "site", "local", 5, data); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var id int64
	if err := database.QueryRow("SELECT id FROM checks_http ORDER BY id DESC LIMIT 1").Scan(&id); err != nil {
		t.Fatalf("id: %v", err)
	}
	if _, err := database.Exec("UPDATE checks_http SET timestamp = ? WHERE id = ?", at.Format("2006-01-02 15:04:05"), id); err != nil {
		t.Fatalf("timestamp: %v", err)
	}
}

// TestBuildResourcePageHydratesOnlyRepresentatives is the memory-property test:
// a long history of identical checks must collapse before hydration, so only the
// latest row and the elision representative are ever loaded in full.
func TestBuildResourcePageHydratesOnlyRepresentatives(t *testing.T) {
	database := newTestDB(t)
	base := dbSource{db: database}
	now := time.Now().UTC().Add(-time.Hour)

	const similarRows = 200
	for i := 0; i < similarRows; i++ {
		insertHTTPAt(t, database, "same body", core.PASS, now.Add(-time.Duration(similarRows-i)*time.Minute))
	}
	insertHTTPAt(t, database, "unique body", core.FAIL, now.Add(similarRows*time.Minute))

	cs := &countingSource{historySource: base}
	page, err := buildResourcePage(&Config{SiteTitle: "T"}, SiteResult{
		Slug: "site", Name: "Site", CheckType: "http", OutpostSlug: "local", OutpostName: "Local",
	}, cs)
	if err != nil {
		t.Fatalf("buildResourcePage: %v", err)
	}

	// Latest (FAIL) + one representative for the collapsed PASS run.
	if cs.loads > 2 {
		t.Errorf("LoadFull called %d times, want <= 2 (latest + one representative)", cs.loads)
	}
	if page.LatestCheck == nil {
		t.Fatal("LatestCheck is nil")
	}
	var markers int
	for _, r := range page.RecentChecks {
		if strings.Contains(r.Elided, "similar") {
			markers++
		}
	}
	if markers != 1 {
		t.Errorf("got %d elision markers, want 1", markers)
	}
	if page.RecentCount != similarRows {
		t.Errorf("RecentCount = %d, want %d", page.RecentCount, similarRows)
	}
}
