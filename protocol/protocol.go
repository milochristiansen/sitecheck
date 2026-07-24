// Package protocol defines the shared wire types and constants used by both sitecheck (core) and scoutpost (outpost).
// It holds result structs, the CheckResult interface, pass-level constants, the JSON-lines wire format, and streaming
// helpers.
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
)

// --- CheckResult interface --------------------------------------------------

// CheckResult is the common interface shared by all check result types.
type CheckResult interface {
	CheckType() string   // "http", "ping", "tcp", "dns", "ssl", "systemd"
	CheckPass() int      // PASS=2, DEGRADED=1, FAIL=0
	CheckFailReason() string
	CheckResponseMS() float64
}

// --- Typed result structs ---------------------------------------------------

// HTTPResult holds the outcome of an HTTP check.
type HTTPResult struct {
	Pass           int
	FailReason     string
	URL            string
	StatusCode     int
	Body           string
	BodySize       int64
	ResponseTimeMS float64
	TLSVersion     string
	RemoteIP       string
	RedirectCount  int
	Error          string
}

func (r HTTPResult) CheckType() string        { return "http" }
func (r HTTPResult) CheckPass() int           { return r.Pass }
func (r HTTPResult) CheckFailReason() string  { return r.FailReason }
func (r HTTPResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// PingResult holds the outcome of an ICMP ping check.
type PingResult struct {
	Pass            int
	FailReason      string
	Host            string
	PacketsSent     int
	PacketsReceived int
	PacketLossPct   float64
	MinMS           float64
	MaxMS           float64
	ResponseTimeMS  float64
	Error           string
}

func (r PingResult) CheckType() string        { return "ping" }
func (r PingResult) CheckPass() int           { return r.Pass }
func (r PingResult) CheckFailReason() string  { return r.FailReason }
func (r PingResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// TCPResult holds the outcome of a TCP connect check.
type TCPResult struct {
	Pass           int
	FailReason     string
	Host           string
	Port           int
	ResponseTimeMS float64
	RemoteIP       string
	Error          string
}

func (r TCPResult) CheckType() string        { return "tcp" }
func (r TCPResult) CheckPass() int           { return r.Pass }
func (r TCPResult) CheckFailReason() string  { return r.FailReason }
func (r TCPResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// DNSResult holds the outcome of a DNS lookup check.
type DNSResult struct {
	Pass           int
	FailReason     string
	Host           string
	IPs            []string
	ResponseTimeMS float64
	Error          string
}

func (r DNSResult) CheckType() string        { return "dns" }
func (r DNSResult) CheckPass() int           { return r.Pass }
func (r DNSResult) CheckFailReason() string  { return r.FailReason }
func (r DNSResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// SSLResult holds the outcome of an SSL certificate check.
type SSLResult struct {
	Pass           int
	FailReason     string
	Host           string
	Port           int
	Issuer         string
	Subject        string
	NotBefore      string
	NotAfter       string
	DaysRemaining  int
	ResponseTimeMS float64
	Error          string
}

func (r SSLResult) CheckType() string        { return "ssl" }
func (r SSLResult) CheckPass() int           { return r.Pass }
func (r SSLResult) CheckFailReason() string  { return r.FailReason }
func (r SSLResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// SystemdResult holds the outcome of a systemd service check.
type SystemdResult struct {
	Pass           int
	FailReason     string
	ServiceName    string
	ActiveState    string
	SubState       string
	LoadState      string
	MainPID        int
	ResponseTimeMS float64
	Error          string
}

func (r SystemdResult) CheckType() string        { return "systemd" }
func (r SystemdResult) CheckPass() int           { return r.Pass }
func (r SystemdResult) CheckFailReason() string  { return r.FailReason }
func (r SystemdResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// OutpostResult holds the health status of an outpost as a whole. It is not produced by Lua scripts — the core
// synthesizes it after each outpost's result stream closes to track outpost response time, check counts, and
// connectivity.
type OutpostResult struct {
	Pass           int
	FailReason     string
	ResponseTimeMS float64
	Error          string
	CheckCount     int
	FailCount      int
}

func (r OutpostResult) CheckType() string        { return "outpost" }
func (r OutpostResult) CheckPass() int           { return r.Pass }
func (r OutpostResult) CheckFailReason() string  { return r.FailReason }
func (r OutpostResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// --- Wire format ------------------------------------------------------------

// WireResult is the JSON-lines wire format sent from outpost to core.
type WireResult struct {
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	Desc            string          `json:"desc"`
	CheckType       string          `json:"check_type"`
	Pass            int             `json:"pass"`
	FailReason      string          `json:"fail_reason,omitempty"`
	ResponseMS      float64         `json:"response_ms"`
	ElapsedMS       int64           `json:"elapsed_ms"`
	Error           string          `json:"error,omitempty"`
	Data            json.RawMessage `json:"data"`
	NotifyPass      bool            `json:"notify_pass"`
	NotifyDegraded  bool            `json:"notify_degraded"`
	NotifyFail      bool            `json:"notify_fail"`
	OutpostSlug     string          `json:"outpost_slug,omitempty"`
}

// NewWireResult converts a typed CheckResult to a WireResult, marshaling the typed data into Data.
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

// ReadResults reads newline-delimited JSON WireResult objects from r, sending each on the returned channel. The channel
// is closed when the reader reaches EOF or encounters an error.
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
