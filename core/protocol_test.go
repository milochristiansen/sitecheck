package core

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNewWireResult(t *testing.T) {
	wr := NewWireResult(
		"test-slug", "Test Name", "A test description",
		"http",
		PASS, "reason",
		123.4, 567,
		"",
		map[string]interface{}{"status": 200},
		true, false, true,
	)

	if wr.Slug != "test-slug" {
		t.Errorf("Slug = %q, want %q", wr.Slug, "test-slug")
	}
	if wr.Name != "Test Name" {
		t.Errorf("Name = %q, want %q", wr.Name, "Test Name")
	}
	if wr.Pass != PASS {
		t.Errorf("Pass = %d, want %d", wr.Pass, PASS)
	}
	if wr.ResponseMS != 123.4 {
		t.Errorf("ResponseMS = %f, want %f", wr.ResponseMS, 123.4)
	}
	if wr.ElapsedMS != 567 {
		t.Errorf("ElapsedMS = %d, want %d", wr.ElapsedMS, 567)
	}
	if wr.NotifyPass != true || wr.NotifyDegraded != false || wr.NotifyFail != true {
		t.Errorf("Notify flags: pass=%v degraded=%v fail=%v", wr.NotifyPass, wr.NotifyDegraded, wr.NotifyFail)
	}

	// Verify Data is valid JSON with the expected field.
	var m map[string]interface{}
	if err := json.Unmarshal(wr.Data, &m); err != nil {
		t.Fatalf("Data is not valid JSON: %v", err)
	}
	if s, ok := m["status"]; !ok {
		t.Errorf("Data missing 'status' key")
	} else if v, ok := s.(float64); !ok || int(v) != 200 {
		t.Errorf("Data status = %v, want 200", s)
	}
}

func TestWriteReadResult(t *testing.T) {
	wr := NewWireResult(
		"slug1", "Name1", "Desc1",
		"tcp",
		DEGRADED, "slow response",
		50.0, 100,
		"",
		map[string]interface{}{"port": 443},
		false, true, false,
	)

	var buf bytes.Buffer
	if err := WriteResult(&buf, wr); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	ch := ReadResults(&buf)
	got := <-ch

	if got.Slug != "slug1" {
		t.Errorf("Slug = %q, want %q", got.Slug, "slug1")
	}
	if got.CheckType != "tcp" {
		t.Errorf("CheckType = %q, want %q", got.CheckType, "tcp")
	}
	if got.Pass != DEGRADED {
		t.Errorf("Pass = %d, want %d", got.Pass, DEGRADED)
	}
	if got.FailReason != "slow response" {
		t.Errorf("FailReason = %q, want %q", got.FailReason, "slow response")
	}
	if got.ResponseMS != 50.0 {
		t.Errorf("ResponseMS = %f, want %f", got.ResponseMS, 50.0)
	}
}

func TestPassConstants(t *testing.T) {
	if FAIL != 0 {
		t.Errorf("FAIL = %d, want 0", FAIL)
	}
	if DEGRADED != 1 {
		t.Errorf("DEGRADED = %d, want 1", DEGRADED)
	}
	if PASS != 2 {
		t.Errorf("PASS = %d, want 2", PASS)
	}
	if UNKNOWN != -1 {
		t.Errorf("UNKNOWN = %d, want -1", UNKNOWN)
	}
}

func TestReadResultsMultiple(t *testing.T) {
	var buf bytes.Buffer
	for i := range 3 {
		wr := NewWireResult(
			"slug"+string(rune('a'+i)), "Name", "",
			"http", PASS, "", 1.0, 2, "", nil,
			true, true, true,
		)
		if err := WriteResult(&buf, wr); err != nil {
			t.Fatalf("WriteResult %d: %v", i, err)
		}
	}

	ch := ReadResults(&buf)
	count := 0
	for range ch {
		count++
	}
	if count != 3 {
		t.Errorf("got %d results, want 3", count)
	}
}

func TestReadResultsEmpty(t *testing.T) {
	ch := ReadResults(bytes.NewReader([]byte{}))
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel for empty input")
	}
}

func TestReadResultsInvalidJSON(t *testing.T) {
	ch := ReadResults(bytes.NewReader([]byte("not json\n")))
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel for invalid JSON")
	}
}

func TestWireResultSitesRoundTrip(t *testing.T) {
	wr := NewWireResult(
		"slug1", "Name1", "Desc1",
		"http", PASS, "", 1.0, 2, "", nil,
		true, true, true,
	)
	wr.Sites = map[string]string{"internal": "basic", "default": "full"}
	wr.Version = WireVersion

	var buf bytes.Buffer
	if err := WriteResult(&buf, wr); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	got := <-ReadResults(&buf)

	if len(got.Sites) != 2 || got.Sites["internal"] != "basic" || got.Sites["default"] != "full" {
		t.Errorf("Sites = %v, want map with internal=basic, default=full", got.Sites)
	}
	if got.Version != WireVersion {
		t.Errorf("Version = %q, want %q", got.Version, WireVersion)
	}
}

func TestWireResultSitesAbsent(t *testing.T) {
	// Absent fields must unmarshal to nil map and empty version (old format).
	var wr WireResult
	if err := json.Unmarshal([]byte(`{"slug":"s","check_type":"http","pass":2}`), &wr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wr.Sites != nil {
		t.Errorf("Sites = %v, want nil for absent field", wr.Sites)
	}
	if wr.Version != "" {
		t.Errorf("Version = %q, want empty for absent field", wr.Version)
	}
}

func TestIsKnownWireVersion(t *testing.T) {
	cases := map[string]bool{
		"":     true, // absent field ⇒ old format, version 1
		"1":    true,
		"1.1":  true,
		"2.0":  false,
		"1.2":  false,
		"9.9":  false,
		"1.10": false,
	}
	for v, want := range cases {
		if got := IsKnownWireVersion(v); got != want {
			t.Errorf("IsKnownWireVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestWireVersionFormat(t *testing.T) {
	if WireVersion != "1.1" {
		t.Errorf("WireVersion = %q, want %q", WireVersion, "1.1")
	}
}
