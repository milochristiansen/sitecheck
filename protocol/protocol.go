// Package protocol defines the shared wire types and constants used by both sitecheck (core) and scoutpost (outpost).
// It holds the CheckResult interface, pass-level constants, the JSON-lines wire format, and streaming helpers.
//
// Concrete result structs live in the checktypes/* plugin packages; they implement CheckResult.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// --- Pass level constants ---------------------------------------------------

// These are exported to Lua scripts by the outpost's Lua VM.
const (
	FAIL     = 0
	DEGRADED = 1
	PASS     = 2

	// CheckTypeLuaError is the sentinel CheckType returned by an outpost when a Lua check script
	// encountered a runtime error. The core resolves the real type from DB history.
	CheckTypeLuaError = "_lua_error"
)

// --- CheckResult interface --------------------------------------------------

// CheckResult is the common interface shared by all check result types.
// Each plugin package defines a concrete result struct implementing this interface.
type CheckResult interface {
	CheckType() string   // e.g. "http", "ping", "tcp", "dns", "ssl", "systemd", "outpost"
	CheckPass() int      // PASS=2, DEGRADED=1, FAIL=0
	CheckFailReason() string
	CheckResponseMS() float64
}

// --- Wire format ------------------------------------------------------------

// WireVersion is the wire format version stamped by scoutpost on every WireResult it emits.
// The version is a string, not a number: JSON floats make minor versions ambiguous ("1.10"
// vs "1.1"), while string comparison keeps the semantics exact.
const WireVersion = "1.1"

// WireResult is the JSON-lines wire format sent from outpost to core.
// Data is the check-type-specific payload serialized as a JSON object.
type WireResult struct {
	Slug            string            `json:"slug"`
	Name            string            `json:"name"`
	Desc            string            `json:"desc"`
	CheckType       string            `json:"check_type"`
	Pass            int               `json:"pass"`
	FailReason      string            `json:"fail_reason,omitempty"`
	ResponseMS      float64           `json:"response_ms"`
	ElapsedMS       int64             `json:"elapsed_ms"`
	Error           string            `json:"error,omitempty"`
	Data            json.RawMessage   `json:"data"`
	NotifyPass      bool              `json:"notify_pass"`
	NotifyDegraded  bool              `json:"notify_degraded"`
	NotifyFail      bool              `json:"notify_fail"`
	OutpostSlug     string            `json:"outpost_slug,omitempty"`
	Sites           map[string]string `json:"sites,omitempty"` // site name → detail level
	Version         string            `json:"version,omitempty"`
}

// IsKnownWireVersion reports whether v is a wire format version this build understands.
// The empty string means the field was absent — the old format, i.e. version 1.
func IsKnownWireVersion(v string) bool {
	switch v {
	case "", "1", "1.1":
		return true
	}
	return false
}

// NewWireResult builds a WireResult from individual fields, marshaling the
// typed data into Data.
func NewWireResult(
	slug, name, desc string,
	checkType string,
	pass int, failReason string,
	responseMS float64, elapsedMS int64,
	errStr string,
	data interface{},
	notifyPass, notifyDegraded, notifyFail bool,
) WireResult {
	var raw json.RawMessage
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	return WireResult{
		Slug:           slug,
		Name:           name,
		Desc:           desc,
		CheckType:      checkType,
		Pass:           pass,
		FailReason:     failReason,
		ResponseMS:     responseMS,
		ElapsedMS:      elapsedMS,
		Error:          errStr,
		Data:           raw,
		NotifyPass:     notifyPass,
		NotifyDegraded: notifyDegraded,
		NotifyFail:     notifyFail,
	}
}

// --- Streaming helpers ------------------------------------------------------

// WriteResult writes a WireResult as a JSON line to w, followed by a newline.
func WriteResult(w io.Writer, r WireResult) error {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

// ReadResults reads newline-delimited JSON WireResult objects from r, sending
// each on the returned channel. The channel is closed when the reader reaches
// EOF or encounters an error.
func ReadResults(r io.Reader) <-chan WireResult {
	ch := make(chan WireResult)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var wr WireResult
			if err := json.Unmarshal(line, &wr); err != nil {
				// Skip unparseable lines; the outpost should never send them.
				continue
			}
			ch <- wr
		}
	}()
	return ch
}
