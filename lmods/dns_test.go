package lmods

import (
	"testing"
)

func TestDNSLookupOK(t *testing.T) {
	l := testState(registerDNS, 5)

	script := `
		function check()
			return dns_lookup("localhost", {timeout = 5})
		end
	`

	r := runLuaCheck(t, l, script).(*DNSResult)

	if r.Error != "" {
		t.Fatalf("Error = %q, want empty", r.Error)
	}
	if len(r.IPs) == 0 {
		t.Error("IPs is empty")
	}
	hasLocal := false
	for _, ip := range r.IPs {
		if ip == "127.0.0.1" || ip == "::1" {
			hasLocal = true
		}
	}
	if !hasLocal {
		t.Errorf("expected localhost IP in %v", r.IPs)
	}
}

func TestDNSLookupBadHost(t *testing.T) {
	l := testState(registerDNS, 2)

	script := `
		function check()
			return dns_lookup("invalid-host-xyz-12345.invalid", {timeout = 2})
		end
	`

	r := runLuaCheck(t, l, script).(*DNSResult)

	if r.Error == "" {
		t.Error("expected error for bad DNS host, got none")
	}
}
