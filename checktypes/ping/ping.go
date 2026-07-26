// Package ping implements registry.CheckPlugin for ICMP ping checks.
package ping

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)

// --- Result struct ----------------------------------------------------------

// PingResult implements protocol.CheckResult for ICMP echo checks.
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

func (r *PingResult) CheckType() string        { return "ping" }
func (r *PingResult) CheckPass() int           { return r.Pass }
func (r *PingResult) CheckFailReason() string  { return r.FailReason }
func (r *PingResult) CheckResponseMS() float64 { return r.ResponseTimeMS }

// --- DB row struct ----------------------------------------------------------

// PingCheck is a single row from checks_ping.
type PingCheck struct {
	ID              int64
	Slug            string
	OutpostSlug     string
	Timestamp       string
	DurationMS      int64
	Pass            int
	ResponseTimeMS  float64
	PacketsSent     int
	PacketsReceived int
	PacketLossPct   float64
	MinMS           float64
	MaxMS           float64
	Host            string
	Error           string
}

// --- Plugin -----------------------------------------------------------------

type pingPlugin struct{}

// --- Identity ---------------------------------------------------------------

func (p *pingPlugin) TypeName() string { return "ping" }

// --- DB schema --------------------------------------------------------------

func (p *pingPlugin) TableName() string { return "checks_ping" }

func (p *pingPlugin) CreateTableDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS checks_ping (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			slug            TEXT NOT NULL,
			outpost_slug    TEXT NOT NULL DEFAULT '',
			timestamp       TEXT DEFAULT (datetime('now')),
			duration_ms     INTEGER,
			pass            INTEGER NOT NULL,
			response_time_ms REAL,
			packets_sent    INTEGER,
			packets_received INTEGER,
			packet_loss_pct REAL,
			min_ms          REAL,
			max_ms          REAL,
			host            TEXT NOT NULL,
			error           TEXT
		)`,
	}
}

func (p *pingPlugin) CreateIndexDDL() []string {
	return []string{
		`CREATE INDEX IF NOT EXISTS idx_checks_ping_slug_time ON checks_ping(slug, timestamp)`,
	}
}

// --- DB operations ----------------------------------------------------------

func (p *pingPlugin) Insert(db *sql.DB, slug, outpostSlug string, elapsedMS int64, data json.RawMessage) error {
	var r PingResult
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("unmarshal ping data: %w", err)
	}
	_, err := db.Exec(
		`INSERT INTO checks_ping
			(slug, outpost_slug, duration_ms, pass, response_time_ms, packets_sent, packets_received, packet_loss_pct, min_ms, max_ms, host, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, r.Pass, r.ResponseTimeMS,
		r.PacketsSent, r.PacketsReceived, r.PacketLossPct, r.MinMS, r.MaxMS,
		r.Host, r.Error,
	)
	if err != nil {
		return fmt.Errorf("insert ping check: %w", err)
	}
	return nil
}

func (p *pingPlugin) InsertError(db *sql.DB, slug, outpostSlug string, elapsedMS int64, pass int, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO checks_ping
			(slug, outpost_slug, duration_ms, pass, host, error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		slug, outpostSlug, elapsedMS, pass, "(error)", errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert ping error: %w", err)
	}
	return nil
}

func (p *pingPlugin) QuerySince(db *sql.DB, slug, outpostSlug string, since time.Time) (interface{}, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(
		`SELECT id, slug, timestamp, duration_ms, pass, response_time_ms,
			packets_sent, packets_received, packet_loss_pct, min_ms, max_ms, host, error
		FROM checks_ping WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp`,
		slug, outpostSlug, sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("query ping checks since: %w", err)
	}
	defer rows.Close()

	var checks []PingCheck
	for rows.Next() {
		var (
			c              PingCheck
			durationMS     sql.NullInt64
			responseMS     sql.NullFloat64
			packetsSent    sql.NullInt64
			packetsRcvd    sql.NullInt64
			packetLossPct  sql.NullFloat64
			minMS          sql.NullFloat64
			maxMS          sql.NullFloat64
			host           sql.NullString
			errMsg         sql.NullString
		)
		err := rows.Scan(&c.ID, &c.Slug, &c.Timestamp, &durationMS, &c.Pass,
			&responseMS, &packetsSent, &packetsRcvd, &packetLossPct,
			&minMS, &maxMS, &host, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ping check since: %w", err)
		}
		c.DurationMS = durationMS.Int64
		c.ResponseTimeMS = responseMS.Float64
		c.PacketsSent = int(packetsSent.Int64)
		c.PacketsReceived = int(packetsRcvd.Int64)
		c.PacketLossPct = packetLossPct.Float64
		c.MinMS = minMS.Float64
		c.MaxMS = maxMS.Float64
		c.Host = host.String
		c.Error = errMsg.String
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// --- Common field access ----------------------------------------------------

func (p *pingPlugin) ExtractPoints(history interface{}) []registry.CheckPoint {
	h, ok := history.([]PingCheck)
	if !ok {
		return nil
	}
	pts := make([]registry.CheckPoint, len(h))
	for i, c := range h {
		pts[i] = registry.CheckPoint{Pass: c.Pass, Resp: c.ResponseTimeMS, TS: c.Timestamp}
	}
	return pts
}

func (p *pingPlugin) ExtractDurationPoints(history interface{}) []registry.CheckPoint {
	return nil
}

func (p *pingPlugin) LatestRecent(history interface{}, maxRecent int) (latest, recent interface{}, count int) {
	h, ok := history.([]PingCheck)
	if !ok || len(h) == 0 {
		return nil, nil, 0
	}
	lat := &h[len(h)-1]
	n := len(h) - 1
	if n > maxRecent {
		n = maxRecent
	}
	rev := make([]PingCheck, n)
	for i := range n {
		rev[i] = h[len(h)-2-i]
	}
	return lat, rev, n
}

// --- Lua registration -------------------------------------------------------

func (p *pingPlugin) RegisterLua(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)

		count := 3
		timeout := defaultTimeout
		privileged := true

		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			count = readIntOpt(l, 2, "count", 3)
			timeout = readIntOpt(l, 2, "timeout", defaultTimeout)
			privileged = readBoolOpt(l, 2, "privileged", true)
		}

		r := &PingResult{
			Pass:        protocol.FAIL,
			Host:        host,
			PacketsSent: count,
		}

		ips, err := net.LookupIP(host)
		if err != nil {
			r.Error = fmt.Sprintf("lookup %s: %v", host, err)
			pushPingResult(l, r)
			return 1
		}

		var target net.UDPAddr
		var useIPv6 bool
		for _, ip := range ips {
			if ip.To4() != nil {
				target = net.UDPAddr{IP: ip}
				useIPv6 = false
				break
			}
			if target.IP == nil && ip.To16() != nil {
				target = net.UDPAddr{IP: ip}
				useIPv6 = true
			}
		}

		if target.IP == nil {
			r.Error = fmt.Sprintf("no suitable IP for %s", host)
			pushPingResult(l, r)
			return 1
		}

		var conn net.PacketConn
		if privileged {
			if useIPv6 {
				conn, err = icmp.ListenPacket("ip6:ipv6-icmp", "::")
			} else {
				conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
			}
		} else {
			if useIPv6 {
				conn, err = icmp.ListenPacket("udp6", "::")
			} else {
				conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
			}
		}
		if err != nil {
			r.Error = fmt.Sprintf("listen icmp: %v", err)
			pushPingResult(l, r)
			return 1
		}
		defer conn.Close()

		conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))

		var rtts []float64
		received := 0

		for seq := range count {
			var msg icmp.Message
			if useIPv6 {
				msg = icmp.Message{
					Type: ipv6.ICMPTypeEchoRequest,
					Code: 0,
					Body: &icmp.Echo{
						ID:   1,
						Seq:  seq,
						Data: []byte("sitecheck-ping"),
					},
				}
			} else {
				msg = icmp.Message{
					Type: ipv4.ICMPTypeEcho,
					Code: 0,
					Body: &icmp.Echo{
						ID:   1,
						Seq:  seq,
						Data: []byte("sitecheck-ping"),
					},
				}
			}

			wb, err := msg.Marshal(nil)
			if err != nil {
				continue
			}

			sendTime := time.Now()
			if _, err := conn.WriteTo(wb, &target); err != nil {
				continue
			}

			reply := make([]byte, 1500)
			_, _, err = conn.ReadFrom(reply)
			if err != nil {
				continue
			}

			rtt := float64(time.Since(sendTime).Microseconds()) / 1000.0
			rtts = append(rtts, rtt)
			received++
		}

		r.PacketsReceived = received
		if count > 0 {
			r.PacketLossPct = float64(count-received) / float64(count) * 100.0
		}

		if received > 0 {
			minRTT := rtts[0]
			maxRTT := rtts[0]
			sum := 0.0
			for _, rt := range rtts {
				sum += rt
				if rt < minRTT {
					minRTT = rt
				}
				if rt > maxRTT {
					maxRTT = rt
				}
			}
			r.MinMS = minRTT
			r.MaxMS = maxRTT
			r.ResponseTimeMS = sum / float64(received)
		}

		pushPingResult(l, r)
		return 1
	})
	l.SetGlobal("icmp_ping")
}

// pushPingResult pushes a PingResult as a Lua userdata with metatable accessors.
func pushPingResult(l *lua.State, r *PingResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*PingResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "Host":
			l.Push(r.Host)
		case "PacketsSent":
			l.Push(int64(r.PacketsSent))
		case "PacketsReceived":
			l.Push(int64(r.PacketsReceived))
		case "PacketLossPct":
			l.Push(r.PacketLossPct)
		case "MinMS":
			l.Push(r.MinMS)
		case "MaxMS":
			l.Push(r.MaxMS)
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "Error":
			pushStr(l, r.Error)
		default:
			l.Push(nil)
		}
		return 1
	})
	l.SetTableRaw(-3)

	l.Push("__newindex")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*PingResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "Host":
			r.Host = l.ToString(3)
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}

// --- Lua helpers (inlined from lmods) ---------------------------------------

func pushStr(l *lua.State, s string) {
	if s == "" {
		l.Push(nil)
	} else {
		l.Push(s)
	}
}

func readIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return def
	}
	switch n := l.GetRaw(-1).(type) {
	case int64:
		l.Pop(1)
		return int(n)
	case float64:
		l.Pop(1)
		return int(n)
	}
	l.Pop(1)
	return def
}

func readBoolOpt(l *lua.State, tableIdx int, key string, def bool) bool {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return def
	}
	if b, ok := l.GetRaw(-1).(bool); ok {
		l.Pop(1)
		return b
	}
	l.Pop(1)
	return def
}

// --- DispatchWireResult -----------------------------------------------------

func (p *pingPlugin) DispatchWireResult(res registry.ResourceMeta, cr protocol.CheckResult, elapsed time.Duration) protocol.WireResult {
	r := cr.(*PingResult)
	return protocol.NewWireResult(
		res.Slug, res.Name, res.Desc,
		"ping", r.Pass, r.FailReason,
		r.ResponseTimeMS, elapsed.Milliseconds(),
		r.Error,
		r,
		res.NotifyPass, res.NotifyDegraded, res.NotifyFail,
	)
}

// --- Templates --------------------------------------------------------------

func (p *pingPlugin) TemplateNames() (row, body string) {
	return "check_ping_row", "check_ping_body"
}

func (p *pingPlugin) TemplateFiles() []string {
	return []string{"templates/checks/ping.html"}
}

// --- Registration -----------------------------------------------------------

func init() {
	registry.Register(&pingPlugin{})
}
