package lmods

// CheckResult is the common interface shared by all check result types.
type CheckResult interface {
	CheckType() string   // "http", "ping", "tcp", "dns", "ssl"
	CheckPass() int      // PASS=2, DEGRADED=1, FAIL=0
	CheckFailReason() string
	CheckResponseMS() float64
}

// --- Result types -----------------------------------------------------------

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

func (r HTTPResult) CheckType() string       { return "http" }
func (r HTTPResult) CheckPass() int          { return r.Pass }
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

func (r PingResult) CheckType() string       { return "ping" }
func (r PingResult) CheckPass() int          { return r.Pass }
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

func (r TCPResult) CheckType() string       { return "tcp" }
func (r TCPResult) CheckPass() int          { return r.Pass }
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

func (r DNSResult) CheckType() string       { return "dns" }
func (r DNSResult) CheckPass() int          { return r.Pass }
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

func (r SSLResult) CheckType() string       { return "ssl" }
func (r SSLResult) CheckPass() int          { return r.Pass }
func (r SSLResult) CheckFailReason() string  { return r.FailReason }
func (r SSLResult) CheckResponseMS() float64 { return r.ResponseTimeMS }
