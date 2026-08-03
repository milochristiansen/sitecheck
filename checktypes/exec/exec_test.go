package exec

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// --- Result interface ---------------------------------------------------------

func TestExecResultInterface(t *testing.T) {
	r := &ExecResult{
		Pass:           2,
		FailReason:     "nope",
		ResponseTimeMS: 42.5,
	}

	if r.CheckType() != "exec" {
		t.Errorf("CheckType = %q, want %q", r.CheckType(), "exec")
	}
	if r.CheckPass() != 2 {
		t.Errorf("CheckPass = %d, want %d", r.CheckPass(), 2)
	}
	if r.CheckFailReason() != "nope" {
		t.Errorf("CheckFailReason = %q, want %q", r.CheckFailReason(), "nope")
	}
	if r.CheckResponseMS() != 42.5 {
		t.Errorf("CheckResponseMS = %v, want %v", r.CheckResponseMS(), 42.5)
	}
}

// --- Plugin identity ----------------------------------------------------------

func TestExecPluginTypeName(t *testing.T) {
	p := &plugin{}
	if p.TypeName() != "exec" {
		t.Errorf("TypeName = %q, want %q", p.TypeName(), "exec")
	}
}

func TestExecPluginTableName(t *testing.T) {
	p := &plugin{}
	if p.TableName() != "checks_exec" {
		t.Errorf("TableName = %q, want %q", p.TableName(), "checks_exec")
	}
}

func TestExecPluginTemplateNames(t *testing.T) {
	p := &plugin{}
	row, body := p.TemplateNames()
	if row != "check_exec_row" {
		t.Errorf("TemplateNames row = %q, want %q", row, "check_exec_row")
	}
	if body != "check_exec_body" {
		t.Errorf("TemplateNames body = %q, want %q", body, "check_exec_body")
	}
}

// --- DDL ---------------------------------------------------------------------

func TestExecPluginCreateTableDDL(t *testing.T) {
	p := &plugin{}
	ddl := p.CreateTableDDL()
	if len(ddl) == 0 {
		t.Fatal("CreateTableDDL returned empty slice")
	}
	if ddl[0] == "" {
		t.Error("CreateTableDDL[0] is empty")
	}
}

func TestExecPluginCreateIndexDDL(t *testing.T) {
	p := &plugin{}
	ddl := p.CreateIndexDDL()
	if len(ddl) == 0 {
		t.Fatal("CreateIndexDDL returned empty slice")
	}
	if ddl[0] == "" {
		t.Error("CreateIndexDDL[0] is empty")
	}
}

// --- DispatchWireResult ------------------------------------------------------

func TestExecPluginDispatchWireResult(t *testing.T) {
	p := &plugin{}
	res := registry.ResourceMeta{
		Slug:           "my-slug",
		Name:           "My Check",
		Desc:           "Does stuff",
		NotifyPass:     true,
		NotifyDegraded: false,
		NotifyFail:     true,
	}
	cr := &ExecResult{
		Pass:           protocol.PASS,
		FailReason:     "",
		ResponseTimeMS: 100.0,
		Command:        "/bin/true",
		ExitCode:       0,
		Stdout:         "ok",
		Stderr:         "",
		Combined:       "ok",
		Error:          "",
	}
	wr := p.DispatchWireResult(res, cr, 200*time.Millisecond)

	if wr.Slug != "my-slug" {
		t.Errorf("wr.Slug = %q, want %q", wr.Slug, "my-slug")
	}
	if wr.Name != "My Check" {
		t.Errorf("wr.Name = %q, want %q", wr.Name, "My Check")
	}
	if wr.Desc != "Does stuff" {
		t.Errorf("wr.Desc = %q, want %q", wr.Desc, "Does stuff")
	}
	if wr.CheckType != "exec" {
		t.Errorf("wr.CheckType = %q, want %q", wr.CheckType, "exec")
	}
	if wr.Pass != protocol.PASS {
		t.Errorf("wr.Pass = %d, want %d", wr.Pass, protocol.PASS)
	}
	if wr.ResponseMS != 100.0 {
		t.Errorf("wr.ResponseMS = %v, want %v", wr.ResponseMS, 100.0)
	}
	if wr.ElapsedMS != 200 {
		t.Errorf("wr.ElapsedMS = %d, want %d", wr.ElapsedMS, 200)
	}
	if wr.Error != "" {
		t.Errorf("wr.Error = %q, want empty", wr.Error)
	}
	if !wr.NotifyPass {
		t.Error("wr.NotifyPass = false, want true")
	}
	if wr.NotifyDegraded {
		t.Error("wr.NotifyDegraded = true, want false")
	}
	if !wr.NotifyFail {
		t.Error("wr.NotifyFail = false, want true")
	}

	// Verify data is valid JSON that decodes to ExecResult
	var decoded ExecResult
	if err := json.Unmarshal(wr.Data, &decoded); err != nil {
		t.Fatalf("wr.Data is not valid JSON: %v", err)
	}
	if decoded.Command != "/bin/true" {
		t.Errorf("decoded.Command = %q, want %q", decoded.Command, "/bin/true")
	}
}

// --- ExtractPoints -----------------------------------------------------------

func TestExecPluginExtractPoints(t *testing.T) {
	p := &plugin{}
	h := []ExecCheck{
		{Pass: protocol.PASS, ResponseTimeMS: 10.0, Timestamp: "2025-01-01 00:00:00.000"},
		{Pass: protocol.FAIL, ResponseTimeMS: 20.0, Timestamp: "2025-01-01 00:01:00.000"},
		{Pass: protocol.DEGRADED, ResponseTimeMS: 30.0, Timestamp: "2025-01-01 00:02:00.000"},
	}
	pts := p.ExtractPoints(h)
	if len(pts) != 3 {
		t.Fatalf("ExtractPoints returned %d points, want 3", len(pts))
	}
	if pts[0].Pass != protocol.PASS || pts[0].Resp != 10.0 || pts[0].TS != "2025-01-01 00:00:00.000" {
		t.Errorf("pts[0] = %+v, want {Pass:2 Resp:10 TS:2025-01-01 00:00:00.000}", pts[0])
	}
	if pts[1].Pass != protocol.FAIL || pts[1].Resp != 20.0 {
		t.Errorf("pts[1] = %+v", pts[1])
	}
	if pts[2].Pass != protocol.DEGRADED || pts[2].Resp != 30.0 {
		t.Errorf("pts[2] = %+v", pts[2])
	}
}

func TestExecPluginExtractPointsInvalidType(t *testing.T) {
	p := &plugin{}
	pts := p.ExtractPoints("not the right type")
	if pts != nil {
		t.Errorf("ExtractPoints with wrong type = %v, want nil", pts)
	}
}

// --- ExtractDurationPoints ---------------------------------------------------

func TestExecPluginExtractDurationPoints(t *testing.T) {
	p := &plugin{}
	pts := p.ExtractDurationPoints(nil)
	if pts != nil {
		t.Errorf("ExtractDurationPoints = %v, want nil", pts)
	}
}

// --- LatestRecent ------------------------------------------------------------

func TestExecPluginLatestRecentEmpty(t *testing.T) {
	p := &plugin{}
	latest, recent, count := p.LatestRecent([]ExecCheck{}, 5)
	if latest != nil {
		t.Errorf("latest = %v, want nil", latest)
	}
	if recent != nil {
		t.Errorf("recent = %v, want nil", recent)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestExecPluginLatestRecentInvalidType(t *testing.T) {
	p := &plugin{}
	latest, recent, count := p.LatestRecent("not a slice", 5)
	if latest != nil {
		t.Errorf("latest = %v, want nil", latest)
	}
	if recent != nil {
		t.Errorf("recent = %v, want nil", recent)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestExecPluginLatestRecentSingle(t *testing.T) {
	p := &plugin{}
	h := []ExecCheck{
		{Pass: protocol.PASS, ResponseTimeMS: 10.0, Timestamp: "2025-01-01 00:00:00.000"},
	}
	latest, recent, count := p.LatestRecent(h, 5)
	if latest == nil {
		t.Fatal("latest is nil, want non-nil")
	}
	l := latest.(*ExecCheck)
	if l.Pass != protocol.PASS {
		t.Errorf("latest.Pass = %d, want %d", l.Pass, protocol.PASS)
	}
	if recent != nil {
		t.Errorf("recent = %v, want nil", recent)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestExecPluginLatestRecentMulti(t *testing.T) {
	p := &plugin{}
	h := []ExecCheck{
		{Pass: protocol.FAIL, ResponseTimeMS: 1.0, Timestamp: "2025-01-01 00:00:00.000"},
		{Pass: protocol.DEGRADED, ResponseTimeMS: 2.0, Timestamp: "2025-01-01 00:01:00.000"},
		{Pass: protocol.PASS, ResponseTimeMS: 3.0, Timestamp: "2025-01-01 00:02:00.000"},
	}
	latest, recent, count := p.LatestRecent(h, 2)
	if latest == nil {
		t.Fatal("latest is nil, want non-nil")
	}
	l := latest.(*ExecCheck)
	if l.Pass != protocol.PASS {
		t.Errorf("latest.Pass = %d, want %d", l.Pass, protocol.PASS)
	}
	if recent == nil {
		t.Fatal("recent is nil, want slice")
	}
	rec := recent.([]ExecCheck)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if len(rec) != 2 {
		t.Fatalf("len(recent) = %d, want 2", len(rec))
	}
	// rec[0] should be the second-last (index 1), rec[1] should be the first (index 0)
	if rec[0].Pass != protocol.DEGRADED {
		t.Errorf("rec[0].Pass = %d, want %d", rec[0].Pass, protocol.DEGRADED)
	}
	if rec[1].Pass != protocol.FAIL {
		t.Errorf("rec[1].Pass = %d, want %d", rec[1].Pass, protocol.FAIL)
	}
}

func TestExecPluginLatestRecentMaxRecentClamp(t *testing.T) {
	p := &plugin{}
	h := []ExecCheck{
		{Pass: 0, Timestamp: "t0"},
		{Pass: 0, Timestamp: "t1"},
		{Pass: 0, Timestamp: "t2"},
		{Pass: 1, Timestamp: "t3"},
	}
	// maxRecent=1 should return at most 1 recent entry
	latest, recent, count := p.LatestRecent(h, 1)
	if latest == nil {
		t.Fatal("latest is nil")
	}
	l := latest.(*ExecCheck)
	if l.Timestamp != "t3" {
		t.Errorf("latest timestamp = %q, want %q", l.Timestamp, "t3")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	rec := recent.([]ExecCheck)
	if len(rec) != 1 {
		t.Fatalf("len(recent) = %d, want 1", len(rec))
	}
	if rec[0].Timestamp != "t2" {
		t.Errorf("rec[0].Timestamp = %q, want %q", rec[0].Timestamp, "t2")
	}
}

// --- truncate ----------------------------------------------------------------

func TestTruncateBelowLimit(t *testing.T) {
	s := "short string"
	got := truncate(s)
	if got != s {
		t.Errorf("truncate(%q) = %q, want %q", s, got, s)
	}
}

func TestTruncateAtLimit(t *testing.T) {
	s := string(make([]byte, maxOutputBytes))
	got := truncate(s)
	if got != s {
		t.Errorf("truncate at limit changed the string (len=%d)", len(got))
	}
}

func TestTruncateAboveLimit(t *testing.T) {
	s := string(make([]byte, maxOutputBytes+100))
	got := truncate(s)
	wantSuffix := "\n…[truncated]"
	expectedLen := maxOutputBytes + len(wantSuffix)
	if len(got) != expectedLen {
		t.Errorf("truncated len = %d, want %d", len(got), expectedLen)
	}
	if got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Errorf("truncated suffix missing, got = %q", got)
	}
}

func TestTruncateAboveLimitPrefix(t *testing.T) {
	// Verify the first bytes are preserved
	s := "hello" + string(make([]byte, maxOutputBytes)) + "world"
	got := truncate(s)
	if got[:5] != "hello" {
		t.Errorf("prefix = %q, want %q", got[:5], "hello")
	}
}

// --- formatCommand -----------------------------------------------------------

func TestFormatCommandNoArgs(t *testing.T) {
	got := formatCommand("ls", nil)
	if got != "ls" {
		t.Errorf("formatCommand(ls, nil) = %q, want %q", got, "ls")
	}
	got = formatCommand("ls", []string{})
	if got != "ls" {
		t.Errorf("formatCommand(ls, []) = %q, want %q", got, "ls")
	}
}

func TestFormatCommandArgsNoSpecialChars(t *testing.T) {
	got := formatCommand("echo", []string{"hello", "world"})
	if got != "echo hello world" {
		t.Errorf("formatCommand = %q, want %q", got, "echo hello world")
	}
}

func TestFormatCommandArgsWithSpaces(t *testing.T) {
	got := formatCommand("cat", []string{"/path/with spaces/file.txt"})
	want := `cat "/path/with spaces/file.txt"`
	if got != want {
		t.Errorf("formatCommand = %q, want %q", got, want)
	}
}

func TestFormatCommandArgsWithDollar(t *testing.T) {
	got := formatCommand("echo", []string{"$HOME"})
	want := `echo "$HOME"`
	if got != want {
		t.Errorf("formatCommand = %q, want %q", got, want)
	}
}

func TestFormatCommandMixedArgs(t *testing.T) {
	got := formatCommand("test", []string{"simple", "with space", "fine"})
	want := `test simple "with space" fine`
	if got != want {
		t.Errorf("formatCommand = %q, want %q", got, want)
	}
}

// --- needsQuote --------------------------------------------------------------

func TestNeedsQuotePlain(t *testing.T) {
	if needsQuote("simple") {
		t.Error("needsQuote('simple') = true, want false")
	}
	if needsQuote("hello123") {
		t.Error("needsQuote('hello123') = true, want false")
	}
	if needsQuote("") {
		t.Error("needsQuote('') = true, want false")
	}
	if needsQuote("under_score") {
		t.Error("needsQuote('under_score') = true, want false")
	}
	if needsQuote("path/to/file") {
		t.Error("needsQuote('path/to/file') = true, want false")
	}
}

func TestNeedsQuoteSpace(t *testing.T) {
	if !needsQuote("with space") {
		t.Error("needsQuote('with space') = false, want true")
	}
}

func TestNeedsQuoteTab(t *testing.T) {
	if !needsQuote("with\ttab") {
		t.Error("needsQuote('with\\ttab') = false, want true")
	}
}

func TestNeedsQuoteNewline(t *testing.T) {
	if !needsQuote("with\nnewline") {
		t.Error("needsQuote('with\\nnewline') = false, want true")
	}
}

func TestNeedsQuoteDoubleQuote(t *testing.T) {
	if !needsQuote(`with"quote`) {
		t.Error("needsQuote('with\"quote') = false, want true")
	}
}

func TestNeedsQuoteSingleQuote(t *testing.T) {
	if !needsQuote(`with'quote`) {
		t.Error("needsQuote(\"with'quote\") = false, want true")
	}
}

func TestNeedsQuoteBackslash(t *testing.T) {
	if !needsQuote(`with\slash`) {
		t.Error("needsQuote('with\\\\slash') = false, want true")
	}
}

func TestNeedsQuoteDollar(t *testing.T) {
	if !needsQuote("$var") {
		t.Error("needsQuote('$var') = false, want true")
	}
}

func TestNeedsQuoteBacktick(t *testing.T) {
	if !needsQuote("`cmd`") {
		t.Error("needsQuote('`cmd`') = false, want true")
	}
}

func TestNeedsQuoteGlobChars(t *testing.T) {
	if !needsQuote("*.go") {
		t.Error("needsQuote('*.go') = false, want true")
	}
	if !needsQuote("[abc]") {
		t.Error("needsQuote('[abc]') = false, want true")
	}
	if !needsQuote("?file") {
		t.Error("needsQuote('?file') = false, want true")
	}
	if !needsQuote("{a,b}") {
		t.Error("needsQuote('{a,b}') = false, want true")
	}
}

func TestNeedsQuoteShellMeta(t *testing.T) {
	if !needsQuote("a|b") {
		t.Error("needsQuote('a|b') = false, want true")
	}
	if !needsQuote("a&b") {
		t.Error("needsQuote('a&b') = false, want true")
	}
	if !needsQuote("a;b") {
		t.Error("needsQuote('a;b') = false, want true")
	}
	if !needsQuote("a<b") {
		t.Error("needsQuote('a<b') = false, want true")
	}
	if !needsQuote("a>b") {
		t.Error("needsQuote('a>b') = false, want true")
	}
	if !needsQuote("a(b") {
		t.Error("needsQuote('a(b') = false, want true")
	}
	if !needsQuote("a)b") {
		t.Error("needsQuote('a)b') = false, want true")
	}
	if !needsQuote("a!b") {
		t.Error("needsQuote('a!b') = false, want true")
	}
	if !needsQuote("#comment") {
		t.Error("needsQuote('#comment') = false, want true")
	}
	if !needsQuote("~home") {
		t.Error("needsQuote('~home') = false, want true")
	}
}

// --- lockedWriter ------------------------------------------------------------

func TestLockedWriterBasic(t *testing.T) {
	var buf bytes.Buffer
	lw := &lockedWriter{w: &buf}
	n, err := lw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("written = %d, want 5", n)
	}
	if buf.String() != "hello" {
		t.Errorf("buf = %q, want %q", buf.String(), "hello")
	}
}

func TestLockedWriterConcurrent(t *testing.T) {
	var mu sync.Mutex
	chunks := make([]string, 0, 10)

	var buf bytes.Buffer
	lw := &lockedWriter{w: &buf}

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := []byte{'A' + byte(i), 'B' + byte(i), 'C' + byte(i)}
			_, err := lw.Write(data)
			if err != nil {
				t.Errorf("write error: %v", err)
				return
			}
			mu.Lock()
			chunks = append(chunks, string(data))
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Total length should be 30 (10 × 3 bytes)
	if buf.Len() != 30 {
		t.Fatalf("total written = %d, want 30", buf.Len())
	}

	// Each 3-byte chunk in the buffer should be one of the expected triples.
	// If writes interleaved, individual bytes wouldn't form valid triples.
	raw := buf.String()
	for i := 0; i < 30; i += 3 {
		chunk := raw[i : i+3]
		if chunk[0] < 'A' || chunk[0] > 'J' || chunk[1] != chunk[0]+1 || chunk[2] != chunk[0]+2 {
			t.Errorf("interleaved write detected at offset %d: got %q", i, chunk)
		}
	}
}
