package auth

import "strings"

// CookieDomainForHost returns "." + domain for the first configured registrable domain the
// host falls under, or "" (host-only) when none match. The empty default keeps a spoofed
// Host header from widening the cookie scope. Any :port is ignored.
func CookieDomainForHost(host string, domains []string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return "." + d
		}
	}
	return ""
}
