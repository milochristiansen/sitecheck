package envcfg

import (
	"os"
	"testing"
)

func TestStr(t *testing.T) {
	const key = "TEST_SITECHECK_STR_ENV"
	defer os.Unsetenv(key)

	// key not set → fallback
	os.Unsetenv(key)
	if got := Str(key, "defaultVal"); got != "defaultVal" {
		t.Errorf("Str unset: want %q, got %q", "defaultVal", got)
	}

	// key set to non-empty → value
	os.Setenv(key, "customValue")
	if got := Str(key, "defaultVal"); got != "customValue" {
		t.Errorf("Str set: want %q, got %q", "customValue", got)
	}

	// key set to empty → fallback
	os.Setenv(key, "")
	if got := Str(key, "defaultVal"); got != "defaultVal" {
		t.Errorf("Str empty: want %q, got %q", "defaultVal", got)
	}
}

func TestInt(t *testing.T) {
	const key = "TEST_SITECHECK_INT_ENV"
	defer os.Unsetenv(key)

	// key not set → fallback
	os.Unsetenv(key)
	if got := Int(key, 42); got != 42 {
		t.Errorf("Int unset: want %d, got %d", 42, got)
	}

	// key set to valid int → parsed value
	os.Setenv(key, "99")
	if got := Int(key, 42); got != 99 {
		t.Errorf("Int valid: want %d, got %d", 99, got)
	}

	// key set to empty string → fallback
	os.Setenv(key, "")
	if got := Int(key, 42); got != 42 {
		t.Errorf("Int empty: want %d, got %d", 42, got)
	}

	// key set to non-int → PANICS
	os.Setenv(key, "not-a-number")
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Int non-int: expected panic but did not panic")
			}
		}()
		Int(key, 42)
	}()
}
