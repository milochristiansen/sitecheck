package main

import (
	"testing"

	"sitecheck/notify"
)

func TestNotifyStatusChange(t *testing.T) {
	// A sender that always fails — tests the decision logic without real HTTP.
	sender := &notify.NtfySender{URL: "http://127.0.0.1:1"}

	t.Run("nil sender returns immediately", func(t *testing.T) {
		notifyStatusChange(nil, statusChange{Slug: "test", Name: "Test Resource", CurrentPass: 2, PrevPass: 2, HasPrev: true})
	})

	t.Run("no previous check returns immediately", func(t *testing.T) {
		notifyStatusChange(sender, statusChange{Slug: "test", Name: "Test Resource", CurrentPass: 2, PrevPass: 2})
	})

	t.Run("same pass returns immediately", func(t *testing.T) {
		notifyStatusChange(sender, statusChange{Slug: "test", Name: "Test Resource", CurrentPass: 2, PrevPass: 2, HasPrev: true})
	})

	t.Run("PASS to FAIL with notifyFail=true calls notify", func(t *testing.T) {
		notifyStatusChange(sender, statusChange{Slug: "test", Name: "Test Resource", FailReason: "timeout", CurrentPass: 0, PrevPass: 2, HasPrev: true, NotifyFail: true})
	})

	t.Run("FAIL to PASS with notifyPass=false prints skip", func(t *testing.T) {
		notifyStatusChange(sender, statusChange{Slug: "test", Name: "Test Resource", CurrentPass: 2, PrevPass: 0, HasPrev: true, NotifyFail: true})
	})

	t.Run("prevPass=-1 does not trigger", func(t *testing.T) {
		notifyStatusChange(sender, statusChange{Slug: "test", Name: "Test Resource", CurrentPass: 2, PrevPass: -1, HasPrev: true, NotifyPass: true})
		notifyStatusChange(sender, statusChange{Slug: "test", Name: "Test Resource", CurrentPass: -1, PrevPass: 0, HasPrev: true, NotifyPass: true})
	})
}
