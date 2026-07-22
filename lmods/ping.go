package lmods

import (
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func registerPing(l *lua.State, defaultTimeout int) {
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
			Pass:        FAIL,
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

		for seq := 0; seq < count; seq++ {
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
		case "PacketsSent":
			r.PacketsSent = int(l.ToInt(3))
		case "PacketsReceived":
			r.PacketsReceived = int(l.ToInt(3))
		case "PacketLossPct":
			r.PacketLossPct = l.ToFloat(3)
		case "MinMS":
			r.MinMS = l.ToFloat(3)
		case "MaxMS":
			r.MaxMS = l.ToFloat(3)
		case "ResponseTimeMS":
			r.ResponseTimeMS = l.ToFloat(3)
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}
