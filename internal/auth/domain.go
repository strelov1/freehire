package auth

import "strings"

// CookieDomainForHost returns the Domain attribute for a session cookie served
// on host, given the configured registrable domains (each bare, like
// "freehire.me"). It returns "." + domain for the first domain the host falls
// under — so the cookie is shared across that domain's subdomains
// (apply./agent. unified SSO) — or "" (host-only) when the host matches none.
//
// The empty default is the safe one: dev/localhost gets a host-only cookie, and
// an unexpected or spoofed Host header can't widen the cookie's scope. Any
// :port on host is ignored.
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
