package main

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// strEnv
// ---------------------------------------------------------------------------

func TestStrEnv(t *testing.T) {
	const key = "TEST_SITECHECK_STR_ENV"
	defer os.Unsetenv(key)

	// key not set → fallback
	os.Unsetenv(key)
	got := strEnv(key, "defaultVal")
	if got != "defaultVal" {
		t.Errorf("strEnv unset: want %q, got %q", "defaultVal", got)
	}

	// key set to non-empty → value
	os.Setenv(key, "customValue")
	got = strEnv(key, "defaultVal")
	if got != "customValue" {
		t.Errorf("strEnv set: want %q, got %q", "customValue", got)
	}

	// key set to empty → fallback
	os.Setenv(key, "")
	got = strEnv(key, "defaultVal")
	if got != "defaultVal" {
		t.Errorf("strEnv empty: want %q, got %q", "defaultVal", got)
	}
}

// ---------------------------------------------------------------------------
// intEnv
// ---------------------------------------------------------------------------

func TestIntEnv(t *testing.T) {
	const key = "TEST_SITECHECK_INT_ENV"
	defer os.Unsetenv(key)

	// key not set → fallback
	os.Unsetenv(key)
	got := intEnv(key, 42)
	if got != 42 {
		t.Errorf("intEnv unset: want %d, got %d", 42, got)
	}

	// key set to valid int → parsed value
	os.Setenv(key, "99")
	got = intEnv(key, 42)
	if got != 99 {
		t.Errorf("intEnv valid: want %d, got %d", 99, got)
	}

	// key set to empty string → fallback
	os.Setenv(key, "")
	got = intEnv(key, 42)
	if got != 42 {
		t.Errorf("intEnv empty: want %d, got %d", 42, got)
	}

	// key set to non-int → PANICS
	os.Setenv(key, "not-a-number")
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("intEnv non-int: expected panic but did not panic")
			}
		}()
		intEnv(key, 42)
	}()
}

// ---------------------------------------------------------------------------
// intSliceEnv
// ---------------------------------------------------------------------------

func TestIntSliceEnv(t *testing.T) {
	const key = "TEST_SITECHECK_INT_SLICE_ENV"
	defer os.Unsetenv(key)

	fallback := []int{10, 20}

	// key not set → fallback
	os.Unsetenv(key)
	got := intSliceEnv(key, fallback)
	if !intSliceEqual(got, fallback) {
		t.Errorf("intSliceEnv unset: want %v, got %v", fallback, got)
	}

	// key set to "1,2,3" → [1,2,3]
	os.Setenv(key, "1,2,3")
	got = intSliceEnv(key, fallback)
	want := []int{1, 2, 3}
	if !intSliceEqual(got, want) {
		t.Errorf("intSliceEnv simple: want %v, got %v", want, got)
	}

	// key set to "  1 , 2 , 3  " → [1,2,3] (whitespace handling)
	os.Setenv(key, "  1 , 2 , 3  ")
	got = intSliceEnv(key, fallback)
	if !intSliceEqual(got, want) {
		t.Errorf("intSliceEnv whitespace: want %v, got %v", want, got)
	}

	// key set to empty string → fallback
	os.Setenv(key, "")
	got = intSliceEnv(key, fallback)
	if !intSliceEqual(got, fallback) {
		t.Errorf("intSliceEnv empty: want %v, got %v", fallback, got)
	}

	// key set to "" after trimming → fallback (all parts are whitespace-only)
	os.Setenv(key, ",")
	got = intSliceEnv(key, fallback)
	if !intSliceEqual(got, fallback) {
		t.Errorf("intSliceEnv empty-after-trim: want %v, got %v", fallback, got)
	}

	// key set to non-int → PANICS
	os.Setenv(key, "1,x,3")
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("intSliceEnv non-int: expected panic but did not panic")
			}
		}()
		intSliceEnv(key, fallback)
	}()
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Config.validate
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	// valid config → nil error
	cfg := &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 10,
		RetentionDays:  7,
		GraphWindows:   []int{24, 168},
	}
	if err := cfg.validate(); err != nil {
		t.Errorf("valid config: unexpected error %v", err)
	}

	// OutpostWorkers = 0 → error
	cfg = &Config{
		OutpostWorkers: 0,
		DefaultTimeout: 10,
		RetentionDays:  7,
		GraphWindows:   []int{24},
	}
	if err := cfg.validate(); err == nil {
		t.Error("OutpostWorkers=0: expected error, got nil")
	}

	// DefaultTimeout = 0 → error
	cfg = &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 0,
		RetentionDays:  7,
		GraphWindows:   []int{24},
	}
	if err := cfg.validate(); err == nil {
		t.Error("DefaultTimeout=0: expected error, got nil")
	}

	// RetentionDays = 0 → error
	cfg = &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 10,
		RetentionDays:  0,
		GraphWindows:   []int{24},
	}
	if err := cfg.validate(); err == nil {
		t.Error("RetentionDays=0: expected error, got nil")
	}

	// GraphWindows empty → error
	cfg = &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 10,
		RetentionDays:  7,
		GraphWindows:   []int{},
	}
	if err := cfg.validate(); err == nil {
		t.Error("GraphWindows empty: expected error, got nil")
	}

	// GraphWindows contains 0 → error
	cfg = &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 10,
		RetentionDays:  7,
		GraphWindows:   []int{0},
	}
	if err := cfg.validate(); err == nil {
		t.Error("GraphWindows contains 0: expected error, got nil")
	}
}
