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
// Config.validate
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	// valid config → nil error
	cfg := &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 10,
		RetentionDays:  7,
	}
	if err := cfg.validate(); err != nil {
		t.Errorf("valid config: unexpected error %v", err)
	}

	// OutpostWorkers = 0 → error
	cfg = &Config{
		OutpostWorkers: 0,
		DefaultTimeout: 10,
		RetentionDays:  7,
	}
	if err := cfg.validate(); err == nil {
		t.Error("OutpostWorkers=0: expected error, got nil")
	}

	// DefaultTimeout = 0 → error
	cfg = &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 0,
		RetentionDays:  7,
	}
	if err := cfg.validate(); err == nil {
		t.Error("DefaultTimeout=0: expected error, got nil")
	}

	// RetentionDays = 0 → error
	cfg = &Config{
		OutpostWorkers: 2,
		DefaultTimeout: 10,
		RetentionDays:  0,
	}
	if err := cfg.validate(); err == nil {
		t.Error("RetentionDays=0: expected error, got nil")
	}
}
