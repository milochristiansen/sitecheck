package main

import (
	"testing"

	"sitecheck/protocol"
)

// ------------------------------------------------------------
// formatTime
// ------------------------------------------------------------

func TestFormatTime(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got := formatTime("2024-01-15 10:30:00")
		want := "2024-01-15 10:30:00 UTC"
		if got != want {
			t.Errorf("formatTime(%q) = %q, want %q", "2024-01-15 10:30:00", got, want)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		got := formatTime("not-a-timestamp")
		if got != "not-a-timestamp" {
			t.Errorf("formatTime(%q) = %q, want input unchanged", "not-a-timestamp", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := formatTime("")
		if got != "" {
			t.Errorf("formatTime(%q) = %q, want empty", "", got)
		}
	})
}

// ------------------------------------------------------------
// formatDuration
// ------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms   float64
		want string
	}{
		{0, "0.0ms"},
		{500, "500.0ms"},
		{1500, "1.50s"},
		{1000, "1.00s"},
		{-500, "-500.0ms"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.ms)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

// ------------------------------------------------------------
// formatDurationMS
// ------------------------------------------------------------

func TestFormatDurationMS(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "0.0ms"},
		{500, "500.0ms"},
		{1500, "1.50s"},
		{1000, "1.00s"},
		{-500, "-500.0ms"},
	}
	for _, tt := range tests {
		got := formatDurationMS(tt.ms)
		if got != tt.want {
			t.Errorf("formatDurationMS(%v) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

// ------------------------------------------------------------
// formatPct
// ------------------------------------------------------------

func TestFormatPct(t *testing.T) {
	tests := []struct {
		v    float64
		want string
	}{
		{99.999, "100.00%"},
		{0, "0.00%"},
		{50, "50.00%"},
		{100.0, "100.00%"},
	}
	for _, tt := range tests {
		got := formatPct(tt.v)
		if got != tt.want {
			t.Errorf("formatPct(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

// ------------------------------------------------------------
// statusClass
// ------------------------------------------------------------

func TestStatusClass(t *testing.T) {
	tests := []struct {
		pass int
		want string
	}{
		{protocol.PASS, "pass"},
		{protocol.DEGRADED, "degraded"},
		{protocol.FAIL, "fail"},
		{-1, "error"},
		{99, "unknown"},
		{-2, "unknown"},
	}
	for _, tt := range tests {
		got := statusClass(tt.pass)
		if got != tt.want {
			t.Errorf("statusClass(%v) = %q, want %q", tt.pass, got, tt.want)
		}
	}
}

// ------------------------------------------------------------
// dict
// ------------------------------------------------------------

func TestDict(t *testing.T) {
	t.Run("valid even args", func(t *testing.T) {
		m, err := dict("key1", "val1", "key2", 42)
		if err != nil {
			t.Fatalf("dict() returned error: %v", err)
		}
		if len(m) != 2 {
			t.Fatalf("dict() = %v, want len 2", m)
		}
		if m["key1"] != "val1" {
			t.Errorf("dict()['key1'] = %v, want %q", m["key1"], "val1")
		}
		if m["key2"] != 42 {
			t.Errorf("dict()['key2'] = %v, want %v", m["key2"], 42)
		}
	})

	t.Run("odd args", func(t *testing.T) {
		_, err := dict("a", "b", "c")
		if err == nil {
			t.Error("dict() with odd args: expected error, got nil")
		}
	})

	t.Run("non-string key", func(t *testing.T) {
		_, err := dict(123, "value")
		if err == nil {
			t.Error("dict() with non-string key: expected error, got nil")
		}
	})

	t.Run("empty", func(t *testing.T) {
		m, err := dict()
		if err != nil {
			t.Fatalf("dict() returned error: %v", err)
		}
		if len(m) != 0 {
			t.Errorf("dict() = %v, want empty map", m)
		}
	})
}

// ------------------------------------------------------------
// calcUptimePct
// ------------------------------------------------------------

func TestCalcUptimePct(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		got := calcUptimePct(nil)
		if got != 0 {
			t.Errorf("calcUptimePct(nil) = %v, want 0", got)
		}
		got = calcUptimePct([]checkPoint{})
		if got != 0 {
			t.Errorf("calcUptimePct([]) = %v, want 0", got)
		}
	})

	t.Run("all PASS", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 2}, {pass: 2}, {pass: 2},
		}
		got := calcUptimePct(pts)
		if got != 100 {
			t.Errorf("calcUptimePct(all PASS) = %v, want 100", got)
		}
	})

	t.Run("all FAIL", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 0}, {pass: 0}, {pass: 0},
		}
		got := calcUptimePct(pts)
		if got != 0 {
			t.Errorf("calcUptimePct(all FAIL) = %v, want 0", got)
		}

		// POSSIBLE BUG: FAIL with non-zero response is still a fail
		pts2 := []checkPoint{
			{pass: 0, resp: 150},
		}
		got2 := calcUptimePct(pts2)
		if got2 != 0 {
			t.Errorf("calcUptimePct(FAIL with response) = %v, want 0", got2)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 2}, {pass: 2}, {pass: 0}, {pass: 2},
		}
		got := calcUptimePct(pts)
		if got != 75 {
			t.Errorf("calcUptimePct(mixed) = %v, want 75", got)
		}
	})

	t.Run("all DEGRADED", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 1}, {pass: 1},
		}
		got := calcUptimePct(pts)
		if got != 100 {
			t.Errorf("calcUptimePct(all DEGRADED) = %v, want 100", got)
		}
	})
}

// ------------------------------------------------------------
// passName
// ------------------------------------------------------------

func TestPassName(t *testing.T) {
	tests := []struct {
		p    int
		want string
	}{
		{protocol.PASS, "PASS"},
		{protocol.DEGRADED, "DEGRADED"},
		{protocol.FAIL, "FAIL"},
		{-1, "UNKNOWN"},
		{99, "UNKNOWN"},
	}
	for _, tt := range tests {
		got := passName(tt.p)
		if got != tt.want {
			t.Errorf("passName(%v) = %q, want %q", tt.p, got, tt.want)
		}
	}
}
