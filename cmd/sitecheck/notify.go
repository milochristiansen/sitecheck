package main

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"sitecheck/protocol"
)

// notify sends a plain-text notification to the ntfy server URL. The URL should be the full URL including the topic,
// e.g. "https://ntfy.sh/mytopic". Returns silently on failure — notifications are best-effort.
func notify(ntfyURL, title, message string) {
	if ntfyURL == "" {
		return
	}

	req, err := http.NewRequest("POST", ntfyURL, bytes.NewBufferString(message))
	if err != nil {
		fmt.Printf("  notify: request error: %v\n", err)
		return
	}
	req.Header.Set("Title", title)
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  notify: send error: %v\n", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("  notify: server returned %d\n", resp.StatusCode)
	}
}

// notifyStatusChange sends a notification if the check status changed from prevPass to currentPass and the notify flag
// for the new status is set. Transitions from UNKNOWN (-1) are never notified — UNKNOWN is a core-internal sentinel for
// outpost connectivity, not a real check result.
func notifyStatusChange(ntfyURL string, slug, name, failReason string, currentPass, prevPass int, hasPrev bool, notifyPass, notifyDegraded, notifyFail bool) {
	if ntfyURL == "" {
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
	notify(ntfyURL, title, message)
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
