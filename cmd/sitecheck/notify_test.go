package main

import (
	"testing"

	"sitecheck/notify"
)

func TestNotifyStatusChange(t *testing.T) {
	// A sender that always fails — tests the decision logic without real HTTP.
	sender := &notify.NtfySender{URL: "http://127.0.0.1:1"}

	t.Run("nil sender returns immediately", func(t *testing.T) {
		notifyStatusChange(nil, "test", "Test Resource", "", 2, 2, true, false, false, false)
	})

	t.Run("no previous check returns immediately", func(t *testing.T) {
		notifyStatusChange(sender, "test", "Test Resource", "", 2, 2, false, false, false, false)
	})

	t.Run("same pass returns immediately", func(t *testing.T) {
		notifyStatusChange(sender, "test", "Test Resource", "", 2, 2, true, false, false, false)
	})

	t.Run("PASS to FAIL with notifyFail=true calls notify", func(t *testing.T) {
		notifyStatusChange(sender, "test", "Test Resource", "timeout", 0, 2, true, false, false, true)
	})

	t.Run("FAIL to PASS with notifyPass=false prints skip", func(t *testing.T) {
		notifyStatusChange(sender, "test", "Test Resource", "", 2, 0, true, false, false, true)
	})

	t.Run("prevPass=-1 does not trigger", func(t *testing.T) {
		notifyStatusChange(sender, "test", "Test Resource", "", 2, -1, true, true, false, false)
		notifyStatusChange(sender, "test", "Test Resource", "", -1, 0, true, true, false, false)
	})
}
