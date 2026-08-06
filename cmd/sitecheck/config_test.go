package main

import (
	"testing"
)

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
