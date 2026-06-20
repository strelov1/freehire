// Package safehttp builds HTTP clients that refuse to connect to non-public
// network addresses. Every outbound fetcher in this service can be pointed at an
// attacker-influenced URL — Telegram posts carry arbitrary links, and orphan-job
// liveness probes URLs that originated from those posts — so a plain http.Client
// is an SSRF primitive: it would happily fetch http://169.254.169.254/ (cloud
// metadata) or an internal RFC1918 service.
//
// The guard runs in the dialer's Control hook, which fires AFTER DNS resolution
// with the concrete IP:port about to be connected. Checking there (rather than the
// hostname in the URL) closes DNS-rebinding: a public-looking host that resolves to
// 127.0.0.1 is rejected at connect time, on the original request and on every
// redirect hop (each hop dials through the same transport).
package safehttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// cgnat is the RFC6598 shared address space (100.64.0.0/10). IsPrivate does not
// cover it, but cloud providers (AWS/GCP) use it for internal/infra addressing and
// it is reachable from instances — so an SSRF guard must block it too.
var cgnat = func() *net.IPNet {
	_, n, _ := net.ParseCIDR("100.64.0.0/10")
	return n
}()

// blocked reports whether ip must not be dialed: loopback, RFC1918/ULA private,
// CGNAT shared space, link-local (which includes the 169.254.169.254 cloud-metadata
// address), multicast, and the unspecified address. nil is treated as blocked (fail
// closed).
func blocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		cgnat.Contains(ip) ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// GuardedDialer returns a net.Dialer whose Control hook rejects a connection to any
// non-public resolved address. The hook receives the post-resolution IP:port, so it
// defeats both direct-IP targets and DNS-rebinding. It is exported so a transport that
// cannot use NewClient/NewTransport (e.g. a tls-client fingerprint client, which accepts
// a net.Dialer via WithDialer) can still dial through the same SSRF guard.
func GuardedDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("safehttp: blocked malformed address %q", address)
			}
			if ip := net.ParseIP(host); ip == nil || blocked(ip) {
				return fmt.Errorf("safehttp: blocked non-public address %s", address)
			}
			return nil
		},
	}
}

// NewTransport returns an *http.Transport that dials through the SSRF guard, with
// sane handshake/idle timeouts so a stalled peer cannot pin a connection open.
func NewTransport(dialTimeout time.Duration) *http.Transport {
	d := GuardedDialer(dialTimeout)
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewClient returns an *http.Client with the SSRF-guarded transport and the given
// overall timeout. Redirects use the default policy (capped at 10 hops); each hop
// re-dials through the guard, so a redirect to an internal address is refused.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTransport(5 * time.Second),
	}
}
