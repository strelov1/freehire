// Package username derives and validates the account-level username: a single,
// unique, user-visible name the hosted mailbox adopts instead of allocating its
// own handle, and that a future talent-network change can expose as a public
// profile URL. It is pure — allocation (picking the first free suffix against
// the store) lives in internal/identity/accounts.
package username

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	minLen = 3
	maxLen = 30
	// fallback is used when a candidate base sanitizes to nothing, or to
	// fewer than minLen characters.
	fallback = "user"
)

var validPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Valid reports whether s is an acceptable username: 3-30 characters, lowercase
// letters and digits with single internal hyphens only, starting and ending
// with a letter or digit.
func Valid(s string) error {
	if len(s) < minLen || len(s) > maxLen {
		return fmt.Errorf("username: must be %d-%d characters", minLen, maxLen)
	}
	if !validPattern.MatchString(s) {
		return fmt.Errorf("username: must be lowercase letters, digits, and single internal hyphens only")
	}
	return nil
}

// Suggest derives a default candidate base from an email's local-part
// (everything before '@'): lowercased, keeping only [a-z0-9-], with '.'
// converted to '-' (the one extra character the legacy mailbox handle format
// allowed). Falls back to a fixed base when the result is empty or shorter
// than the minimum length, so a candidate is always ready for Candidate's
// collision search.
func Suggest(email string) string {
	local := email
	if at := strings.IndexByte(local, '@'); at >= 0 {
		local = local[:at]
	}
	return Sanitize(local)
}

// Sanitize normalizes an arbitrary string (an email local-part, or an existing
// legacy handle) into a valid username base: lowercased, '.' mapped to '-',
// every other disallowed character dropped, consecutive hyphens collapsed,
// leading/trailing hyphens trimmed, truncated to the maximum length, and
// falling back to a fixed base when the result is empty or too short.
func Sanitize(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-':
			b.WriteRune(r)
		case r == '.':
			b.WriteByte('-')
		}
	}

	base := collapseHyphens(b.String())
	base = strings.Trim(base, "-")

	if len(base) > maxLen {
		base = strings.Trim(base[:maxLen], "-")
	}
	if len(base) < minLen {
		return fallback
	}
	return base
}

// collapseHyphens replaces every run of consecutive '-' with a single one.
func collapseHyphens(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if r == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Candidate returns the nth username for a base: the base itself for n<=1,
// then "base-2", "base-3", … so a collision gets the smallest free suffix.
func Candidate(base string, n int) string {
	if n <= 1 {
		return base
	}
	return base + "-" + strconv.Itoa(n)
}

// reserved are usernames nobody may claim, regardless of whether the name is
// otherwise unclaimed: RFC 2142 role addresses and CA/Browser Forum
// constructed email addresses (ported from the mailbox feature this package
// replaces — see internal/application/mailbox's former reservedHandles for
// the rationale), plus product-identity terms a *public* profile name must
// not impersonate.
var reserved = map[string]struct{}{
	// CA/Browser Forum constructed email addresses — certificate issuance.
	"admin": {}, "administrator": {}, "webmaster": {}, "hostmaster": {}, "postmaster": {},
	// RFC 2142 role addresses.
	"abuse": {}, "noc": {}, "security": {}, "usenet": {}, "news": {}, "www": {},
	"uucp": {}, "ftp": {}, "marketing": {}, "sales": {}, "support": {}, "info": {},
	// Bounce and automated-sender names.
	"mailer-daemon": {}, "noreply": {}, "no-reply": {}, "bounce": {}, "bounces": {},
	// Domain-control and mail-authentication reporting.
	"dmarc": {}, "dkim": {}, "spf": {}, "root": {}, "ssl-admin": {}, "ssladmin": {},
	// Product-identity terms — a public profile name must not impersonate freehire itself.
	"freehire": {}, "official": {}, "staff": {}, "moderator": {},
}

// IsReserved reports whether a candidate username is one nobody may claim.
// Matching is on the exact string: a suffixed form like "postmaster-2" or
// "freehire-fan" is not itself an operational address or an impersonation of
// the reserved term, so nothing stops a user from holding it — Candidate's
// collision search relies on this to keep making progress past a reserved
// base.
func IsReserved(s string) bool {
	_, ok := reserved[s]
	return ok
}
