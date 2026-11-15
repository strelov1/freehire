package oauth2server

import (
	"fmt"
	"net/url"
	"strings"
)

// RedirectURIAllowed reports whether candidate matches a registered redirect_uri: exact
// equality, except a loopback redirect (127.0.0.1, [::1], localhost) ignores the port
// because native clients bind an ephemeral one (RFC 8252 §7.3).
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

// maxRedirectURILength bounds a registered redirect_uri so an unauthenticated
// registration cannot park payloads in oauth_clients.
const maxRedirectURILength = 2048

// blockedRedirectSchemes are schemes a browser interprets itself, so a redirect there
// never reaches a client and only serves script or local-file abuse.
var blockedRedirectSchemes = map[string]bool{
	"javascript": true, "data": true, "vbscript": true, "file": true, "blob": true, "about": true,
}

// ValidateRedirectURI decides whether a redirect_uri may be registered (RFC 7591). Three
// shapes are admitted (RFC 8252):
//
//   - https on any host;
//   - http on a loopback host only, since plain http elsewhere would put the code on the wire;
//   - a private-use app scheme (cursor://…).
//
// Host ownership is the consent screen's job (see oauthConsentPage). A fragment is
// refused (RFC 6749 §3.1.2).
func ValidateRedirectURI(raw string) error {
	if len(raw) > maxRedirectURILength {
		return fmt.Errorf("redirect_uri exceeds %d characters", maxRedirectURILength)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("redirect_uri %q is not an absolute URI", raw)
	}
	// Checked on the raw string: url.Parse reports an empty fragment ("…/cb#") as none.
	if strings.Contains(raw, "#") {
		return fmt.Errorf("redirect_uri %q must not carry a fragment", raw)
	}
	switch scheme := strings.ToLower(u.Scheme); {
	case scheme == "https":
		if u.Hostname() == "" {
			return fmt.Errorf("redirect_uri %q has no host", raw)
		}
	case scheme == "http":
		if !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("redirect_uri %q: plain http is allowed only on a loopback host", raw)
		}
	case blockedRedirectSchemes[scheme]:
		return fmt.Errorf("redirect_uri scheme %q is not allowed", u.Scheme)
	}
	return nil
}
