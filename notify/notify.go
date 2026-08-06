// Package notify provides a generic notification transport layer.
// Transports (Ntfy, Telegram, SMTP, etc.) implement the Sender interface.
// Decision logic for when to notify lives in the sitecheck command.
package notify

import (
	"errors"
	"fmt"
)

// Message is a notification to be delivered by a Sender.
type Message struct {
	Title string
	Body  string
}

// Sender delivers notifications via a specific transport.
type Sender interface {
	Send(msg Message) error
}

// Broadcast fans out each message to all configured senders.
// Errors from individual senders are collected and joined;
// a single failure does not prevent other senders from running.
// An empty Broadcast is a no-op.
type Broadcast struct {
	Senders []Sender
}

// Send delivers msg to every sender in the broadcast group.
func (b *Broadcast) Send(msg Message) error {
	var errs []error
	for _, s := range b.Senders {
		if err := s.Send(msg); err != nil {
			errs = append(errs, fmt.Errorf("%T: %w", s, err))
		}
	}
	return errors.Join(errs...)
}
