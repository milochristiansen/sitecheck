package main

import (
	"fmt"

	"sitecheck/core"
	"sitecheck/notify"
)

// statusChange describes a check's status transition for notification.
type statusChange struct {
	Slug                                   string
	Name                                   string
	FailReason                             string
	CurrentPass                            int
	PrevPass                               int
	HasPrev                                bool
	NotifyPass, NotifyDegraded, NotifyFail bool
}

// notifyStatusChange sends a notification if the check status changed from
// PrevPass to CurrentPass and the notify flag for the new status is set.
// Transitions from UNKNOWN are never notified — UNKNOWN is a core-internal
// sentinel for outpost connectivity, not a real check result.
func notifyStatusChange(sender notify.Sender, sc statusChange) {
	if sender == nil || !sc.HasPrev || sc.PrevPass == sc.CurrentPass {
		return
	}

	send := false
	switch sc.CurrentPass {
	case core.PASS:
		send = sc.NotifyPass
	case core.DEGRADED:
		send = sc.NotifyDegraded
	case core.FAIL:
		send = sc.NotifyFail
	}

	if !send {
		fmt.Printf("  notify: %s transition %s→%s, notify flag not set\n",
			sc.Slug, passName(sc.PrevPass), passName(sc.CurrentPass))
		return
	}

	title := fmt.Sprintf("SiteCheck: %s (%s)", sc.Name, sc.Slug)
	message := fmt.Sprintf("%s is now %s", sc.Name, passName(sc.CurrentPass))
	if sc.FailReason != "" && sc.CurrentPass != core.PASS {
		message += ": " + sc.FailReason
	}
	fmt.Printf("  notify: %s transition %s→%s, sending\n",
		sc.Slug, passName(sc.PrevPass), passName(sc.CurrentPass))
	if err := sender.Send(notify.Message{Title: title, Body: message}); err != nil {
		fmt.Printf("  notify: send error: %v\n", err)
	}
}

func passName(p int) string {
	switch p {
	case core.PASS:
		return "PASS"
	case core.DEGRADED:
		return "DEGRADED"
	case core.FAIL:
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}
