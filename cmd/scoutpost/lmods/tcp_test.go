package lmods

import (
	"fmt"
	"net"
	"testing"
	"sitecheck/protocol"
)

func TestTCPConnectOK(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())

	l := testState(registerTCP, 5)

	script := fmt.Sprintf(`
		function check()
			return tcp_connect("127.0.0.1", %s, {timeout = 5})
		end
	`, port)

	r := runLuaCheck(t, l, script).(*protocol.TCPResult)

	if r.Error != "" {
		t.Errorf("Error = %q, want empty", r.Error)
	}
	if r.ResponseTimeMS <= 0 {
		t.Errorf("ResponseTimeMS = %v, want > 0", r.ResponseTimeMS)
	}
}

func TestTCPConnectRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	_, port, _ := net.SplitHostPort(addr)

	l := testState(registerTCP, 5)

	script := fmt.Sprintf(`
		function check()
			return tcp_connect("127.0.0.1", %s, {timeout = 2})
		end
	`, port)

	r := runLuaCheck(t, l, script).(*protocol.TCPResult)

	if r.Error == "" {
		t.Error("expected error for refused connection, got none")
	}
}

func TestTCPConnectBadHost(t *testing.T) {
	l := testState(registerTCP, 2)

	script := `
		function check()
			return tcp_connect("invalid-host-xyz-12345.invalid", 80, {timeout = 2})
		end
	`

	r := runLuaCheck(t, l, script).(*protocol.TCPResult)

	if r.Error == "" {
		t.Error("expected error for bad host, got none")
	}
}
