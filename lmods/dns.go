package lmods

import (
	"context"
	"net"
	"time"

	"github.com/milochristiansen/lua"
)

func registerDNS(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)

		timeout := defaultTimeout
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			timeout = readIntOpt(l, 2, "timeout", defaultTimeout)
		}

		r := &DNSResult{
			
			Pass: FAIL,
			Host: host,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		start := time.Now()
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushDNSResult(l, r)
			return 1
		}

		r.IPs = ips
		pushDNSResult(l, r)
		return 1
	})
	l.SetGlobal("dns_lookup")
}

func pushDNSResult(l *lua.State, r *DNSResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*DNSResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "Host":
			l.Push(r.Host)
		case "IPs":
			pushStrSlice(l, r.IPs)
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
		r := l.ToUser(1).(*DNSResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "Host":
			r.Host = l.ToString(3)
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
