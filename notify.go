package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sitecheck/lmods"
)
// notify sends a plain-text notification to an ntfy topic.
// server is the base URL (e.g. "https://ntfy.sh"); topic is the topic name.
// Returns nil even on failure — notifications are best-effort.
func notify(server, topic, title, message string) {
	if server == "" || topic == "" {
		return
	}
	url := strings.TrimRight(server, "/") + "/" + topic

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(message))
	if err != nil {
		fmt.Printf("  notify: %s (%s) request error: %v\n", topic, title, err)
		return
	}
	req.Header.Set("Title", title)
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  notify: %s (%s) send error: %v\n", topic, title, err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("  notify: %s (%s) server returned %d\n", topic, title, resp.StatusCode)
	}
}

// notifyStatusChange sends a notification if the check status changed from prevPass
// to currentPass and a topic is configured for the new status.
func notifyStatusChange(server string, r Result, currentPass, prevPass int, hasPrev bool) {
	if !hasPrev {
		return
	}
	if prevPass == currentPass {
		return
	}

	var topic string
	switch currentPass {
	case lmods.PASS:
		topic = r.NotifyPass
	case lmods.DEGRADED:
		topic = r.NotifyDegraded
	case lmods.FAIL:
		topic = r.NotifyFail
	}

	if topic == "" {
		fmt.Printf("  notify: %s transition %s→%s but no topic configured for %s\n",
			r.Slug, passName(prevPass), passName(currentPass), passName(currentPass))
		return
	}

	title := fmt.Sprintf("SiteCheck: %s (%s)", r.Name, r.Slug)
	message := fmt.Sprintf("%s is now %s", r.Name, passName(currentPass))
	fmt.Printf("  notify: %s transition %s→%s, sending to %s/%s\n",
		r.Slug, passName(prevPass), passName(currentPass), server, topic)
	notify(server, topic, title, message)
}
