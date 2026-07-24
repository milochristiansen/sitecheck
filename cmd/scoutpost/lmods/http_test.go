package lmods

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milochristiansen/lua"
	"github.com/milochristiansen/lua/lmodbase"
	"sitecheck/protocol"
)

func testState(register func(*lua.State, int), timeout int) *lua.State {
	l := lua.NewState()
	l.Output = io.Discard

	err := l.Protect(func() {
		l.Push(lmodbase.Open)
		l.Call(0, 0)

		l.Push(int64(protocol.PASS))
		l.SetGlobal("PASS")
		l.Push(int64(protocol.DEGRADED))
		l.SetGlobal("DEGRADED")
		l.Push(int64(protocol.FAIL))
		l.SetGlobal("FAIL")

		register(l, timeout)
	})
	if err != nil {
		panic(err)
	}
	return l
}

func runLuaCheck(t *testing.T, l *lua.State, script string) interface{} {
	t.Helper()

	err := l.Protect(func() {
		if e := l.LoadText(strings.NewReader(script), "test.lua", 0); e != nil {
			panic(e)
		}
		l.Call(0, 0)
	})
	if err != nil {
		t.Fatalf("load script: %v", err)
	}

	l.Push("check")
	l.GetTableRaw(lua.GlobalsIndex)
	if err := l.PCall(0, 1); err != nil {
		t.Fatalf("call check(): %v", err)
	}

	if l.TypeOf(-1) != lua.TypUserData {
		t.Fatalf("check() returned %v, want userdata", l.TypeOf(-1))
	}
	raw := l.ToUser(-1)
	l.Pop(1)
	return raw
}

// --- HTTP tests ---

func TestHTTPFetchOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	l := testState(registerHTTP, 5)

	script := fmt.Sprintf(`
		function check()
			return http_fetch("%s", {timeout = 5})
		end
	`, srv.URL)

	r := runLuaCheck(t, l, script).(*protocol.HTTPResult)

	if r.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", r.StatusCode)
	}
	if r.BodySize != 11 {
		t.Errorf("BodySize = %d, want 11", r.BodySize)
	}
	if r.Error != "" {
		t.Errorf("Error = %q, want empty", r.Error)
	}
}

func TestHTTPFetchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	l := testState(registerHTTP, 5)

	script := fmt.Sprintf(`
		function check()
			return http_fetch("%s", {timeout = 5})
		end
	`, srv.URL)

	r := runLuaCheck(t, l, script).(*protocol.HTTPResult)

	if r.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", r.StatusCode)
	}
	if r.Pass != protocol.FAIL {
		t.Errorf("Pass = %d, want FAIL (0)", r.Pass)
	}
}

func TestHTTPFetchConnectionRefused(t *testing.T) {
	l := testState(registerHTTP, 2)

	script := `
		function check()
			return http_fetch("http://127.0.0.1:19999", {timeout = 2})
		end
	`

	r := runLuaCheck(t, l, script).(*protocol.HTTPResult)

	if r.Error == "" {
		t.Error("expected error for connection refused, got none")
	}
}

func TestHTTPFetchRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("redirected"))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	l := testState(registerHTTP, 5)

	script := fmt.Sprintf(`
		function check()
			return http_fetch("%s", {timeout = 5, follow_redirects = true})
		end
	`, redirector.URL)

	r := runLuaCheck(t, l, script).(*protocol.HTTPResult)

	if r.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200 after redirect", r.StatusCode)
	}
	if r.RedirectCount < 1 {
		t.Errorf("RedirectCount = %d, want >= 1", r.RedirectCount)
	}
}

func TestHTTPFetchRedirectDisabled(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("final"))
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	l := testState(registerHTTP, 5)

	script := fmt.Sprintf(`
		function check()
			return http_fetch("%s", {timeout = 5, follow_redirects = false})
		end
	`, redirector.URL)

	r := runLuaCheck(t, l, script).(*protocol.HTTPResult)

	if r.StatusCode != 301 {
		t.Errorf("StatusCode = %d, want 301", r.StatusCode)
	}
}

func TestHTTPFetchCustomHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	l := testState(registerHTTP, 5)

	script := fmt.Sprintf(`
		function check()
			return http_fetch("%s", {
				timeout = 5,
				headers = {["X-Custom"] = "test-value"},
			})
		end
	`, srv.URL)

	r := runLuaCheck(t, l, script).(*protocol.HTTPResult)

	if r.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", r.StatusCode)
	}
	if gotHeader != "test-value" {
		t.Errorf("X-Custom header = %q, want 'test-value'", gotHeader)
	}
}
