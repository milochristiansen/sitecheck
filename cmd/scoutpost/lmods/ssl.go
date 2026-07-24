package lmods

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/protocol"
)

func registerSSL(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		host := l.ToString(1)
		port := 443
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypNumber {
			port = int(l.ToInt(2))
		}

		timeout := defaultTimeout
		insecureSkipVerify := false
		if !l.IsNil(3) && l.TypeOf(3) == lua.TypTable {
			timeout = readIntOpt(l, 3, "timeout", defaultTimeout)
			insecureSkipVerify = readBoolOpt(l, 3, "insecure_skip_verify", false)
		}

		r := &protocol.SSLResult{
			
			Pass: protocol.FAIL,
			Host: host,
			Port: port,
		}

		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		dialer := &net.Dialer{Timeout: time.Duration(timeout) * time.Second}

		start := time.Now()
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		})
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushSSLResult(l, r)
			return 1
		}
		defer conn.Close()

		certs := conn.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			r.Error = "no certificates presented"
			pushSSLResult(l, r)
			return 1
		}

		cert := certs[0]
		r.Subject = cert.Subject.String()
		r.Issuer = cert.Issuer.String()
		r.NotBefore = cert.NotBefore.Format(time.RFC3339)
		r.NotAfter = cert.NotAfter.Format(time.RFC3339)
		r.DaysRemaining = int(time.Until(cert.NotAfter).Hours() / 24)

		pushSSLResult(l, r)
		return 1
	})
	l.SetGlobal("ssl_certificate")
}

func pushSSLResult(l *lua.State, r *protocol.SSLResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*protocol.SSLResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "Host":
			l.Push(r.Host)
		case "Port":
			l.Push(int64(r.Port))
		case "Issuer":
			pushStr(l, r.Issuer)
		case "Subject":
			pushStr(l, r.Subject)
		case "NotBefore":
			pushStr(l, r.NotBefore)
		case "NotAfter":
			pushStr(l, r.NotAfter)
		case "DaysRemaining":
			l.Push(int64(r.DaysRemaining))
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
		r := l.ToUser(1).(*protocol.SSLResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "Host":
			r.Host = l.ToString(3)
		case "Port":
			r.Port = int(l.ToInt(3))
		case "Issuer":
			r.Issuer = l.ToString(3)
		case "Subject":
			r.Subject = l.ToString(3)
		case "NotBefore":
			r.NotBefore = l.ToString(3)
		case "NotAfter":
			r.NotAfter = l.ToString(3)
		case "DaysRemaining":
			r.DaysRemaining = int(l.ToInt(3))
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
