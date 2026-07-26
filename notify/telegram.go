package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TelegramSender delivers notifications via the Telegram Bot API.
// Token is the bot token from @BotFather. Channel is the target chat ID
// (numeric ID, @channelname, or group ID).
type TelegramSender struct {
	Token   string
	Channel string
	Client  *http.Client
}

// Send delivers a notification to the configured Telegram chat.
// The title is rendered as bold HTML; body follows as plain text.
func (t *TelegramSender) Send(msg Message) error {
	if t.Token == "" || t.Channel == "" {
		return nil
	}
	// Escape HTML special chars in title and body, then build message.
	text := "<b>" + escapeHTML(msg.Title) + "</b>\n\n" + escapeHTML(msg.Body)

	body := telegramRequest{
		ChatID:    t.Channel,
		Text:      text,
		ParseMode: "HTML",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := "https://api.telegram.org/bot" + t.Token + "/sendMessage"
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Try to read the Telegram error description.
		var tgErr struct {
			Description string `json:"description"`
		}
		if json.NewDecoder(resp.Body).Decode(&tgErr) == nil && tgErr.Description != "" {
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, tgErr.Description)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

type telegramRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// escapeHTML escapes <, >, and & for Telegram's HTML parse mode.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
