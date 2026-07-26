// Package notify provides a generic notification transport layer.
// Transports (Ntfy, Telegram, SMTP, etc.) implement the Sender interface.
// Decision logic for when to notify lives in the sitecheck command.
package notify

import "fmt"

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
	if len(errs) == 0 {
		return nil
	}
	// Join errors into one; fmt.Errorf("%w") unwrapping is not needed
	// for logging — callers just print the error string.
	return &broadcastError{errs: errs}
}

// broadcastError aggregates errors from multiple senders.
type broadcastError struct {
	errs []error
}

func (e *broadcastError) Error() string {
	s := "broadcast errors:"
	for _, err := range e.errs {
		s += "\n  - " + err.Error()
	}
	return s
}
