package oauth2server

import "net/url"

// RedirectURIAllowed reports whether candidate may be used for this authorize
// request, given the client's registered redirect_uris. Matching is exact string
// equality, with one exception: a loopback redirect (127.0.0.1, [::1], or
// localhost) matches on scheme+host+path alone, ignoring the port — native MCP
// clients bind an ephemeral local port per request and cannot register it in
// advance (RFC 8252 §7.3).
func RedirectURIAllowed(registered []string, candidate string) bool {
	for _, r := range registered {
		if r == candidate {
			return true
		}
		if loopbackMatch(r, candidate) {
			return true
		}
	}
	return false
}

func loopbackMatch(registered, candidate string) bool {
	a, errA := url.Parse(registered)
	b, errB := url.Parse(candidate)
	if errA != nil || errB != nil {
		return false
	}
	if a.Scheme != b.Scheme || a.Path != b.Path || a.RawQuery != "" || b.RawQuery != "" {
		return false
	}
	return isLoopbackHost(a.Hostname()) && a.Hostname() == b.Hostname()
}

func isLoopbackHost(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
