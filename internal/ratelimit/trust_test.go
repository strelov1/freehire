package ratelimit

import (
	"net"
	"testing"
)

// TestTrusted_OurSSRAssertsNoAddress is the case production actually presents, and the case
// the middleware test could not reach.
//
// Fiber resolves c.IP() to what a TRUSTED peer asserted, not to the socket it came from. Our
// SSR asserts nothing, so c.IP() is the empty string — which parsed to nil and read as
// "untrusted". Every server-rendered page in the site then shared one key, `publicread:ip:`,
// and one 600/minute budget, so the busiest pages answered 429 while the limiter's whole
// purpose was to exempt them.
func TestTrusted_OurSSRAssertsNoAddress(t *testing.T) {
	if !trusted("", net.ParseIP("127.0.0.1")) {
		t.Error("a loopback peer asserting no client address is our own SSR, and must not be counted")
	}
	if !trusted("", net.ParseIP("10.1.2.3")) {
		t.Error("a private-network peer asserting no client address must not be counted either")
	}
}

// TestTrusted_AnUnclaimedRequestFromOutsideIsStillCounted: the API listens on 0.0.0.0, so a
// stranger can reach it directly and assert nothing at all. The socket cannot be forged, which
// is exactly why the fallback reads it rather than the header.
func TestTrusted_AnUnclaimedRequestFromOutsideIsStillCounted(t *testing.T) {
	if trusted("", net.ParseIP("203.0.113.7")) {
		t.Error("a public peer asserting no client address is a stranger, not our SSR")
	}
	if trusted("", nil) {
		t.Error("an unparseable peer is not trusted — an unrecognizable caller is the one to count")
	}
}

// TestTrusted_AClaimedAddressStillDecides preserves what already worked: nginx asserts the
// visitor's address, and the visitor is who gets counted — never the loopback socket nginx
// happens to arrive on.
func TestTrusted_AClaimedAddressStillDecides(t *testing.T) {
	if trusted("203.0.113.7", net.ParseIP("127.0.0.1")) {
		t.Error("a visitor proxied by nginx must be counted, not exempted for arriving on loopback")
	}
	if !trusted("127.0.0.1", net.ParseIP("127.0.0.1")) {
		t.Error("a peer asserting loopback is still trusted")
	}
}
