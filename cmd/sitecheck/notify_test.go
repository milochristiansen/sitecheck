package main

import (
	"testing"
)

func TestNotifyStatusChange(t *testing.T) {
	bogusURL := "http://127.0.0.1:1"

	t.Run("empty URL returns immediately", func(t *testing.T) {
		notifyStatusChange("", "test", "Test Resource", "", 2, 2, true, false, false, false)
	})

	t.Run("no previous check returns immediately", func(t *testing.T) {
		notifyStatusChange(bogusURL, "test", "Test Resource", "", 2, 2, false, false, false, false)
	})

	t.Run("same pass returns immediately", func(t *testing.T) {
		notifyStatusChange(bogusURL, "test", "Test Resource", "", 2, 2, true, false, false, false)
	})

	t.Run("PASS to FAIL with notifyFail=true calls notify", func(t *testing.T) {
		// notify() will attempt a real HTTP call and fail non-fatally; just verify no panic
		notifyStatusChange(bogusURL, "test", "Test Resource", "timeout", 0, 2, true, false, false, true)
	})

	t.Run("FAIL to PASS with notifyPass=false prints skip", func(t *testing.T) {
		// Should reach the !send branch and print a skip message, not call notify
		notifyStatusChange(bogusURL, "test", "Test Resource", "", 2, 0, true, false, false, true)
	})

	t.Run("prevPass=-1 does not trigger", func(t *testing.T) {
		notifyStatusChange(bogusURL, "test", "Test Resource", "", 2, -1, true, true, false, false)
		notifyStatusChange(bogusURL, "test", "Test Resource", "", -1, 0, true, true, false, false)
	})
}
