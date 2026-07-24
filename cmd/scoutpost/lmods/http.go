package lmods

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/milochristiansen/lua"
	"sitecheck/protocol"
)

func registerHTTP(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		url := l.ToString(1)

		method := "GET"
		timeout := defaultTimeout
		followRedirects := true
		maxRedirects := 10
		insecureSkipVerify := false
		var headers map[string]string
		var body string

		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			method = readStringOpt(l, 2, "method", "GET")
			timeout = readIntOpt(l, 2, "timeout", defaultTimeout)
			followRedirects = readBoolOpt(l, 2, "follow_redirects", true)
			maxRedirects = readIntOpt(l, 2, "max_redirects", 10)
			insecureSkipVerify = readBoolOpt(l, 2, "insecure_skip_verify", false)
			headers = readStringMapOpt(l, 2, "headers")
			body = readStringOpt(l, 2, "body", "")
		}

		r := &protocol.HTTPResult{
			Pass: protocol.FAIL,
			URL:  url,
		}

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		}

		redirectCount := 0

		client := &http.Client{
			Timeout:   time.Duration(timeout) * time.Second,
			Transport: transport,
		}

		if !followRedirects {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		} else {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				redirectCount = len(via)
				if len(via) >= maxRedirects {
					return fmt.Errorf("too many redirects (%d)", maxRedirects)
				}
				return nil
			}
		}

		var reqBody io.Reader
		if body != "" {
			reqBody = strings.NewReader(body)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			r.Error = err.Error()
			pushHTTPResult(l, r)
			return 1
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushHTTPResult(l, r)
			return 1
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

		r.StatusCode = resp.StatusCode
		r.Body = string(bodyBytes)
		r.BodySize = int64(len(bodyBytes))
		r.RedirectCount = redirectCount

		if resp.TLS != nil {
			r.TLSVersion = tlsVersionName(resp.TLS.Version)
		}

		if resp.Request != nil && resp.Request.URL != nil {
			r.URL = resp.Request.URL.String()
		}

		if resp.Request != nil && resp.Request.URL != nil {
			host := resp.Request.URL.Hostname()
			if ips, err := net.LookupHost(host); err == nil && len(ips) > 0 {
				r.RemoteIP = ips[0]
			}
		}

		pushHTTPResult(l, r)
		return 1
	})
	l.SetGlobal("http_fetch")
}

// pushHTTPResult pushes *protocol.HTTPResult as userdata with an explicit metatable.
func pushHTTPResult(l *lua.State, r *protocol.HTTPResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*protocol.HTTPResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "URL":
			l.Push(r.URL)
		case "StatusCode":
			l.Push(int64(r.StatusCode))
		case "BodySize":
			l.Push(r.BodySize)
		case "Body":
			l.Push(r.Body)
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "TLSVersion":
			pushStr(l, r.TLSVersion)
		case "RemoteIP":
			pushStr(l, r.RemoteIP)
		case "RedirectCount":
			l.Push(int64(r.RedirectCount))
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
		r := l.ToUser(1).(*protocol.HTTPResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "URL":
			r.URL = l.ToString(3)
		case "StatusCode":
			r.StatusCode = int(l.ToInt(3))
		case "BodySize":
			r.BodySize = l.ToInt(3)
		case "Body":
			r.Body = l.ToString(3)
		case "ResponseTimeMS":
			r.ResponseTimeMS = l.ToFloat(3)
		case "TLSVersion":
			r.TLSVersion = l.ToString(3)
		case "RemoteIP":
			r.RemoteIP = l.ToString(3)
		case "RedirectCount":
			r.RedirectCount = int(l.ToInt(3))
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}

func pushStr(l *lua.State, s string) {
	if s == "" {
		l.Push(nil)
	} else {
		l.Push(s)
	}
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("TLS 0x%04x", v)
	}
}
