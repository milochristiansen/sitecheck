package lmods

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"sitecheck/protocol"
)

func TestSSLCertificateOK(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := srv.Listener.Addr().(*net.TCPAddr).IP.String()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	l := testState(registerSSL, 5)

	script := fmt.Sprintf(`
		function check()
			return ssl_certificate("%s", %d, {timeout = 5, insecure_skip_verify = true})
		end
	`, host, port)

	r := runLuaCheck(t, l, script).(*protocol.SSLResult)

	if r.Error != "" {
		t.Fatalf("Error = %q, want empty", r.Error)
	}
	if r.Issuer == "" {
		t.Error("Issuer should not be empty")
	}
	if r.Subject == "" {
		t.Error("Subject should not be empty")
	}
	if r.DaysRemaining == 0 {
		t.Errorf("DaysRemaining = %d, want non-zero", r.DaysRemaining)
	}
	if r.NotBefore == "" {
		t.Error("NotBefore should not be empty")
	}
	if r.NotAfter == "" {
		t.Error("NotAfter should not be empty")
	}
}

func TestSSLCertificateBadHost(t *testing.T) {
	l := testState(registerSSL, 2)

	script := `
		function check()
			return ssl_certificate("invalid-host-xyz-12345.invalid", 443, {timeout = 2})
		end
	`

	r := runLuaCheck(t, l, script).(*protocol.SSLResult)

	if r.Error == "" {
		t.Error("expected error for bad SSL host, got none")
	}
}
