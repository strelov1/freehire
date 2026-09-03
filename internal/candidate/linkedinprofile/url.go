package linkedinprofile

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
)

// ErrNotAProfileURL means the input does not name a LinkedIn member profile. It is
// returned before any outbound request is made, so a caller that hands this package
// a hostile string has not caused a fetch of anything.
var ErrNotAProfileURL = errors.New("linkedinprofile: not a LinkedIn member profile URL")

const (
	profileHost = "www.linkedin.com"
	profileBase = "https://" + profileHost
	// A public id is a URL path segment. LinkedIn's own are well under this; the
	// bound exists so a pathological input cannot become a pathological request.
	maxPublicIDLen = 128
)

// publicID validates a user-supplied LinkedIn profile reference and returns the
// member's public id, or ErrNotAProfileURL.
//
// It accepts what a user actually pastes: the address bar (which may carry a
// sub-page), the share menu (which appends tracking parameters), a country
// subdomain, a missing scheme, or just the id. All of those name one profile, so all
// of them reduce to the one thing that identifies it, and nothing downstream has to
// know which form arrived.
//
// Reducing to the id rather than to a URL is what makes the validation impossible to
// skip: there is no other way to build a request in this package, so a hostile string
// cannot reach the network by taking a different route.
//
// The host check is an exact match against a small set, never a suffix test —
// "linkedin.com.evil.example" ends in nothing this accepts, and that is the whole
// point of matching the host rather than searching the string.
func publicID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrNotAProfileURL
	}

	// A bare public id carries no host and no path, so there is nothing to parse.
	if !strings.Contains(input, "/") && !strings.Contains(input, ":") && !strings.Contains(input, ".") {
		return checkedID(input)
	}

	// A pasted host with no scheme is not a URL to url.Parse — it reads as a path.
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}

	u, err := url.Parse(input)
	if err != nil {
		return "", ErrNotAProfileURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrNotAProfileURL
	}
	if !isProfileHost(u.Hostname()) {
		return "", ErrNotAProfileURL
	}

	// Everything after the public id — /details/experience/, /recent-activity/ — is a
	// view of the same profile, and the query is tracking. Only the id identifies it.
	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(segments) < 2 || segments[0] != "in" {
		return "", ErrNotAProfileURL
	}
	id, err := url.PathUnescape(segments[1])
	if err != nil {
		return "", ErrNotAProfileURL
	}
	return checkedID(id)
}

// isProfileHost matches linkedin.com, www.linkedin.com, and the two-letter country
// subdomains LinkedIn serves the same profile from.
func isProfileHost(host string) bool {
	host = strings.ToLower(host)
	switch host {
	case "linkedin.com", profileHost:
		return true
	}
	sub, ok := strings.CutSuffix(host, ".linkedin.com")
	if !ok || len(sub) != 2 {
		return false
	}
	return isASCIILetters(sub)
}

// checkedID accepts the shape of a LinkedIn public id: letters (LinkedIn issues
// non-ASCII ones for non-Latin names), digits, hyphens and underscores. Rejecting
// everything else is what stops a hostname, a path or a query from being mistaken
// for an id when the input arrived with no host at all.
//
// The id comes back lowercased. LinkedIn issues them lowercase and treats them
// case-insensitively, so folding here is what makes "one profile, one request" true of a
// user who typed their own name with a capital.
func checkedID(id string) (string, error) {
	if id == "" || len(id) > maxPublicIDLen {
		return "", ErrNotAProfileURL
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return "", ErrNotAProfileURL
	}
	return strings.ToLower(id), nil
}

func isASCIILetters(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}
