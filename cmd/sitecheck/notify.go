package main

import (
	"fmt"

	"sitecheck/notify"
	"sitecheck/protocol"
)

// notifyStatusChange sends a notification if the check status changed from prevPass to currentPass and the notify flag
// for the new status is set. Transitions from UNKNOWN (-1) are never notified — UNKNOWN is a core-internal sentinel for
// outpost connectivity, not a real check result.
func notifyStatusChange(sender notify.Sender, slug, name, failReason string, currentPass, prevPass int, hasPrev bool, notifyPass, notifyDegraded, notifyFail bool) {
	if sender == nil {
		return
	}
	if !hasPrev {
		return
	}
	if prevPass == currentPass {
		return
	}

	send := false
	switch currentPass {
	case protocol.PASS:
		send = notifyPass
	case protocol.DEGRADED:
		send = notifyDegraded
	case protocol.FAIL:
		send = notifyFail
	}

	if !send {
		fmt.Printf("  notify: %s transition %s→%s, notify flag not set\n",
			slug, passName(prevPass), passName(currentPass))
		return
	}

	title := fmt.Sprintf("SiteCheck: %s (%s)", name, slug)
	message := fmt.Sprintf("%s is now %s", name, passName(currentPass))
	if failReason != "" && currentPass != protocol.PASS {
		message += ": " + failReason
	}
	fmt.Printf("  notify: %s transition %s→%s, sending\n",
		slug, passName(prevPass), passName(currentPass))
	if err := sender.Send(notify.Message{Title: title, Body: message}); err != nil {
		fmt.Printf("  notify: send error: %v\n", err)
	}
}

func passName(p int) string {
	switch p {
	case protocol.PASS:
		return "PASS"
	case protocol.DEGRADED:
		return "DEGRADED"
	case protocol.FAIL:
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}
