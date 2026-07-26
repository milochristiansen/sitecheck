package notify

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

// NtfySender delivers notifications via an Ntfy server (https://ntfy.sh).
// URL is the full server+topic URL, e.g. "https://ntfy.sh/mytopic".
type NtfySender struct {
	URL    string
	Client *http.Client
}

// Send posts a plain-text notification to the configured Ntfy topic.
// The title is sent as the Title header; body is the POST body.
func (n *NtfySender) Send(msg Message) error {
	if n.URL == "" {
		return nil
	}
	req, err := http.NewRequest("POST", n.URL, bytes.NewBufferString(msg.Body))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Title", msg.Title)
	req.Header.Set("Content-Type", "text/plain")

	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
