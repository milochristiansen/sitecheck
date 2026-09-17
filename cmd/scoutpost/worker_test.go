package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/milochristiansen/lua"

	"sitecheck/cmd/scoutpost/lmods"
	"sitecheck/core"
)

func TestTitleCase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"single word lowercase", "hello", "Hello"},
		{"single word UPPERCASE", "HELLO", "Hello"},
		{"multiple words with spaces", "hello world example", "Hello World Example"},
		{"already title case", "Hello World", "Hello World"},
		{"words with numbers", "hello2 world3", "Hello2 World3"},
		{"leading/trailing spaces", "  hello world  ", "Hello World"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titleCase(tt.input)
			if got != tt.want {
				t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToRegistryMeta(t *testing.T) {
	t.Run("copies all fields correctly", func(t *testing.T) {
		r := Resource{
			Slug:           "test-check",
			ScriptPath:     "/some/path.lua",
			Name:           "My Check",
			Desc:           "A test check description",
			Skip:           true,
			NotifyPass:     true,
			NotifyDegraded: false,
			NotifyFail:     true,
		}
		got := r.toRegistryMeta()
		want := core.ResourceMeta{
			Slug:           "test-check",
			Name:           "My Check",
			Desc:           "A test check description",
			NotifyPass:     true,
			NotifyDegraded: false,
			NotifyFail:     true,
		}
		if got != want {
			t.Errorf("toRegistryMeta() = %+v, want %+v", got, want)
		}
	})

	t.Run("zero-value Resource produces zero-value ResourceMeta", func(t *testing.T) {
		r := Resource{}
		got := r.toRegistryMeta()
		want := core.ResourceMeta{}
		if got != want {
			t.Errorf("toRegistryMeta() on zero Resource = %+v, want %+v", got, want)
		}
	})
}

// writeTestScript writes script to a temp file and returns its path.
func writeTestScript(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "check.lua")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// loadTestCheck writes script to a temp file, loads it into a fresh Lua state
// and returns both the state and a Resource describing the script. It is used
// by the retry tests below.
func loadTestCheck(t *testing.T, script string) (*lua.State, Resource) {
	t.Helper()
	path := writeTestScript(t, script)
	l, err := lmods.NewState(5)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if err := lmods.ExecuteFile(l, path); err != nil {
		t.Fatalf("ExecuteFile: %v", err)
	}
	return l, Resource{Slug: "test-check", ScriptPath: path, Name: "Test Check"}
}

// httpCheckScript returns a check() that maps HTTP responses onto pass levels.
// Error/2xx are handled the same way as the example scripts; 503 becomes
// DEGRADED and any other status becomes FAIL, with the response body recorded
// as the failure reason so tests can tell attempts apart.
func httpCheckScript(url string) string {
	return fmt.Sprintf(`
function check()
	local r = http_fetch(%q, { timeout = 5 })
	if r.Error ~= "" then
		return r
	end
	if r.StatusCode == 200 then
		r.Pass = PASS
	elseif r.StatusCode == 503 then
		r.Pass = DEGRADED
		r.FailReason = "degraded " .. r.Body
	else
		r.Pass = FAIL
		r.FailReason = r.Body
	end
	return r
end
`, url)
}

func TestCheckPassed(t *testing.T) {
	tests := []struct {
		name string
		wr   core.WireResult
		want bool
	}{
		{"pass", core.WireResult{Pass: core.PASS}, true},
		{"pass with empty error", core.WireResult{Pass: core.PASS, Error: ""}, true},
		{"pass with error", core.WireResult{Pass: core.PASS, Error: "boom"}, false},
		{"degraded", core.WireResult{Pass: core.DEGRADED}, false},
		{"fail", core.WireResult{Pass: core.FAIL}, false},
		{"unknown", core.WireResult{Pass: core.UNKNOWN}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkPassed(tt.wr); got != tt.want {
				t.Errorf("checkPassed(%+v) = %t, want %t", tt.wr, got, tt.want)
			}
		})
	}
}

func TestRunCheckWithRetryReturnsPassingRetry(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "first attempt")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	l, res := loadTestCheck(t, httpCheckScript(srv.URL))
	wr := RunCheckWithRetry(l, res)

	if wr.Pass != core.PASS {
		t.Fatalf("Pass = %d, want PASS (%d); error=%q fail_reason=%q", wr.Pass, core.PASS, wr.Error, wr.FailReason)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("server saw %d request(s), want 2", got)
	}
}

func TestRunCheckWithRetryReturnsSecondResultOnRepeatedFailure(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "attempt-%d", n)
	}))
	defer srv.Close()

	l, res := loadTestCheck(t, httpCheckScript(srv.URL))
	wr := RunCheckWithRetry(l, res)

	if wr.Pass != core.FAIL {
		t.Fatalf("Pass = %d, want FAIL (%d)", wr.Pass, core.FAIL)
	}
	if wr.FailReason != "attempt-2" {
		t.Fatalf("FailReason = %q, want %q (the result must come from the second attempt)", wr.FailReason, "attempt-2")
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("server saw %d request(s), want 2", got)
	}
}

func TestRunCheckWithRetryDoesNotRetryPass(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	l, res := loadTestCheck(t, httpCheckScript(srv.URL))
	wr := RunCheckWithRetry(l, res)

	if wr.Pass != core.PASS {
		t.Fatalf("Pass = %d, want PASS (%d)", wr.Pass, core.PASS)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("server saw %d request(s), want 1 (passing checks must not be retried)", got)
	}
}

func TestRunCheckWithRetryRetriesDegraded(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "flaky")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	l, res := loadTestCheck(t, httpCheckScript(srv.URL))
	wr := RunCheckWithRetry(l, res)

	if wr.Pass != core.PASS {
		t.Fatalf("Pass = %d, want PASS (%d); fail_reason=%q", wr.Pass, core.PASS, wr.FailReason)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("server saw %d request(s), want 2", got)
	}
}

func TestRunCheckDefaultsToRetry(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	l, res := loadTestCheck(t, httpCheckScript(srv.URL))
	wr := RunCheck(l, res)

	if wr.Pass != core.PASS {
		t.Fatalf("Pass = %d, want PASS (%d)", wr.Pass, core.PASS)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("server saw %d request(s), want 2", got)
	}
}

func TestRunCheckWithRetryAfterLuaError(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// check() raises a Lua error on the first call, then succeeds. This makes
	// sure a caught error leaves the Lua state usable for the retry.
	script := fmt.Sprintf(`
n = 0
function check()
	n = n + 1
	if n == 1 then
		error("transient lua error")
	end
	local r = http_fetch(%q, { timeout = 5 })
	r.Pass = PASS
	return r
end
`, srv.URL)

	l, res := loadTestCheck(t, script)
	wr := RunCheckWithRetry(l, res)

	if wr.Pass != core.PASS {
		t.Fatalf("Pass = %d, want PASS (%d); error=%q", wr.Pass, core.PASS, wr.Error)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("server saw %d request(s), want 1 (only the second attempt should reach the server)", got)
	}
}

// TestPoolRetriesFailedCheck exercises the real worker path: a script that fails
// on its first run should be retried by the pool, and only the retry's result
// should be emitted.
func TestPoolRetriesFailedCheck(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := writeTestScript(t, httpCheckScript(srv.URL))
	pool := NewPool(1, 5)
	go func() {
		pool.Submit(Job{Resource: Resource{Slug: "test-check", ScriptPath: path, Name: "Test Check"}})
		pool.Wait()
	}()

	var results []core.WireResult
	for wr := range pool.Results() {
		results = append(results, wr)
	}
	if len(results) != 1 {
		t.Fatalf("got %d result(s), want 1", len(results))
	}
	if results[0].Pass != core.PASS {
		t.Fatalf("Pass = %d, want PASS (%d)", results[0].Pass, core.PASS)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("server saw %d request(s), want 2", got)
	}
}
