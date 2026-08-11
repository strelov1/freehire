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
	"net/url"
	"syscall"
	"time"

	"golang.org/x/oauth2"
)

// specialPurpose are IANA special-purpose IPv4 blocks that net.IP.IsPrivate does not
// cover (it only implements RFC1918 for v4 and RFC4193 ULA for v6). None of these are
// publicly routable, but a deployment network can still forward to them, so the SSRF
// guard must deny them explicitly rather than rely on the caller-facing "any
// non-public address" policy being satisfied by IsPrivate alone.
var specialPurpose = func() []*net.IPNet {
	cidrs := []string{
		"100.64.0.0/10",   // RFC6598 CGNAT shared space (also cloud infra addressing)
		"192.0.0.0/24",    // RFC6890 IETF protocol assignments
		"192.0.2.0/24",    // RFC5737 documentation (TEST-NET-1)
		"198.18.0.0/15",   // RFC2544 benchmarking
		"198.51.100.0/24", // RFC5737 documentation (TEST-NET-2)
		"203.0.113.0/24",  // RFC5737 documentation (TEST-NET-3)
		"240.0.0.0/4",     // RFC1112 reserved (includes 255.255.255.255 limited broadcast)
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(fmt.Sprintf("safehttp: bad CIDR %q: %v", c, err))
		}
		nets = append(nets, n)
	}
	return nets
}()

// blocked reports whether ip must not be dialed: loopback, RFC1918/ULA private,
// link-local (which includes the 169.254.169.254 cloud-metadata address), multicast,
// unspecified, and the IANA special-purpose blocks in specialPurpose. nil is treated
// as blocked (fail closed).
func blocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	for _, n := range specialPurpose {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
	return NewTransportWithProxy(dialTimeout, nil)
}

// NewTransportWithProxy is NewTransport with an optional egress proxy. When proxy is
// non-nil the transport routes requests through it; the guarded dialer then vets the
// PROXY's IP (which must be public), not the ultimate target — the proxy resolves that
// itself. So a proxied transport must only be pointed at trusted hosts, never at
// attacker-influenced URLs (liveness/linksource keep the nil-proxy transport). A nil
// proxy yields exactly the direct, target-guarded behavior of NewTransport.
func NewTransportWithProxy(dialTimeout time.Duration, proxy *url.URL) *http.Transport {
	d := GuardedDialer(dialTimeout)
	t := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxy != nil {
		t.Proxy = http.ProxyURL(proxy)
	}
	return t
}

// NewClient returns an *http.Client with the SSRF-guarded transport and the given
// overall timeout. Redirects use the default policy (capped at 10 hops); each hop
// re-dials through the guard, so a redirect to an internal address is refused.
func NewClient(timeout time.Duration) *http.Client {
	return NewClientWithProxy(timeout, nil)
}

// NewClientWithProxy is NewClient with an optional egress proxy (see
// NewTransportWithProxy for the SSRF caveat). A nil proxy is identical to NewClient.
func NewClientWithProxy(timeout time.Duration, proxy *url.URL) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTransportWithProxy(5*time.Second, proxy),
	}
}

// GuardedOAuth2Context returns ctx carrying a guarded client under oauth2's own
// context key, so golang.org/x/oauth2 (Exchange, TokenSource, Client, and any
// provider call that reads its HTTP client from ctx) dials through this package's
// guard instead of the library's zero-value default client.
//
// A caller-supplied oauth2.HTTPClient is respected and returned unchanged — tests
// inject an httptest client for their loopback stub, and this must not override
// that with a guarded client that would reject 127.0.0.1.
func GuardedOAuth2Context(ctx context.Context, timeout time.Duration) context.Context {
	if _, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, NewClient(timeout))
}
