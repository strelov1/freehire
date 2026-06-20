package safehttp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBlockedClassifiesAddresses(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback v6
		{"10.0.0.5", true},        // RFC1918
		{"172.16.3.4", true},      // RFC1918
		{"192.168.1.1", true},     // RFC1918
		{"169.254.169.254", true}, // cloud metadata (link-local)
		{"169.254.1.1", true},     // link-local
		{"100.64.0.1", true},      // CGNAT / RFC6598 (cloud infra addressing)
		{"100.127.255.255", true}, // CGNAT upper bound
		{"0.0.0.0", true},         // unspecified
		{"fc00::1", true},         // unique local v6
		{"8.8.8.8", false},        // public
		{"1.1.1.1", false},        // public
		{"93.184.216.34", false},  // public
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("bad test ip %q", tc.ip)
		}
		if got := blocked(ip); got != tc.want {
			t.Errorf("blocked(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestClientRefusesLoopback(t *testing.T) {
	// A server on 127.0.0.1 represents an internal target an SSRF would try to reach.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(5 * time.Second)
	_, err := client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected the guarded client to refuse a loopback target, got nil error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error %q does not mention the block", err)
	}
}

func TestClientReachesPublicViaControl(t *testing.T) {
	// A dialer whose Control passes (public IP) must still connect. We can't bind a
	// public IP locally, so assert the guard lets a public address through by calling
	// the control hook directly via a custom resolver-free dial to a public literal.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// Dial a public, non-routable-in-test address: it should fail with a network
	// error (timeout/unreachable), NOT the safehttp block error.
	d := GuardedDialer(200 * time.Millisecond)
	_, err := d.DialContext(ctx, "tcp", "8.8.8.8:9")
	if err != nil && strings.Contains(err.Error(), "blocked") {
		t.Errorf("public address was blocked: %v", err)
	}
}

// TestGuardedDialerRefusesLoopback covers the exported GuardedDialer directly: it is the
// SSRF seam reused by transports that bypass NewClient (e.g. the Meta tls-client client,
// which dials through it via WithDialer). A loopback target must be refused at connect time.
func TestGuardedDialerRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parse test server addr: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = GuardedDialer(200*time.Millisecond).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", port))
	if err == nil {
		t.Fatal("expected GuardedDialer to refuse a loopback target, got nil error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error %q does not mention the block", err)
	}
}
