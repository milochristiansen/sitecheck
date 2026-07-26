package notify

import (
	"errors"
	"testing"
)

// recordingSender captures sent messages for inspection.
type recordingSender struct {
	sent []Message
	err  error
}

func (r *recordingSender) Send(msg Message) error {
	r.sent = append(r.sent, msg)
	return r.err
}

func TestNtfySender(t *testing.T) {
	t.Run("empty URL is a no-op", func(t *testing.T) {
		s := &NtfySender{URL: ""}
		if err := s.Send(Message{Title: "T", Body: "B"}); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		s := &NtfySender{URL: "http://127.0.0.1:1"}
		err := s.Send(Message{Title: "T", Body: "B"})
		if err == nil {
			t.Error("expected error from invalid URL, got nil")
		}
	})
}

func TestTelegramSender(t *testing.T) {
	t.Run("empty token is a no-op", func(t *testing.T) {
		s := &TelegramSender{Token: "", Channel: "@test"}
		if err := s.Send(Message{Title: "T", Body: "B"}); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("empty channel is a no-op", func(t *testing.T) {
		s := &TelegramSender{Token: "token", Channel: ""}
		if err := s.Send(Message{Title: "T", Body: "B"}); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("invalid token returns error", func(t *testing.T) {
		s := &TelegramSender{Token: "bogus", Channel: "@test"}
		err := s.Send(Message{Title: "T", Body: "B"})
		if err == nil {
			t.Error("expected error from invalid token, got nil")
		}
	})
}

func TestBroadcast(t *testing.T) {
	t.Run("empty broadcast is a no-op", func(t *testing.T) {
		b := &Broadcast{}
		if err := b.Send(Message{Title: "T", Body: "B"}); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("fans out to all senders", func(t *testing.T) {
		r1 := &recordingSender{}
		r2 := &recordingSender{}
		b := &Broadcast{Senders: []Sender{r1, r2}}
		err := b.Send(Message{Title: "T", Body: "B"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(r1.sent) != 1 || len(r2.sent) != 1 {
			t.Errorf("expected 1 msg each, got r1=%d r2=%d", len(r1.sent), len(r2.sent))
		}
	})

	t.Run("one failure does not block others", func(t *testing.T) {
		r1 := &recordingSender{err: errors.New("boom")}
		r2 := &recordingSender{}
		b := &Broadcast{Senders: []Sender{r1, r2}}
		err := b.Send(Message{Title: "T", Body: "B"})
		if err == nil {
			t.Error("expected error from failing sender")
		}
		if len(r2.sent) != 1 {
			t.Errorf("r2 should have received message, got %d", len(r2.sent))
		}
	})
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a < b", "a &lt; b"},
		{"a > b", "a &gt; b"},
		{"a & b", "a &amp; b"},
		{"<b>bold</b>", "&lt;b&gt;bold&lt;/b&gt;"},
	}
	for _, tt := range tests {
		if got := escapeHTML(tt.in); got != tt.want {
			t.Errorf("escapeHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
