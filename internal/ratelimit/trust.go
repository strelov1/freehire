package ratelimit

import (
	"net"

	"github.com/gofiber/fiber/v2"
)

// TrustedCIDRs is the set of peers allowed to speak on another caller's behalf,
// and therefore the set this package does not defend against.
//
// It is the single definition: cmd/server hands it to Fiber as TrustedProxies so
// an X-Real-IP from one of these is believed, and Middleware reads it to decide
// whom not to count. Defining it twice would let the two drift, and a peer we
// trust to assert someone else's address while also rate-limiting as if it were
// a stranger is incoherent in both directions.
//
// The concrete need is the front end: SSR reaches the API at
// http://127.0.0.1:8081, bypassing nginx and forwarding no client-address
// header, so every server-rendered page presents the same peer. Counting those
// would put the whole site in one caller's budget.
var TrustedCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.1/32",
}

// trustedNets is TrustedCIDRs parsed once at init. A malformed entry is a
// programming error in a package-level literal, so it panics here rather than
// silently narrowing the trusted set at runtime — a CIDR that fails to parse
// would otherwise turn our own SSR into a rate-limited stranger.
var trustedNets = func() []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(TrustedCIDRs))
	for _, cidr := range TrustedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("ratelimit: malformed trusted CIDR " + cidr + ": " + err.Error())
		}
		nets = append(nets, network)
	}
	return nets
}()

// trustedPeer reports whether the request comes from a peer we do not rate-limit.
// An address that does not parse is treated as untrusted: an unrecognizable
// caller is exactly the one to keep counting.
func trustedPeer(c *fiber.Ctx) bool {
	ip := net.ParseIP(c.IP())
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, network := range trustedNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
