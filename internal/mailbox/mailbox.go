// Package mailbox derives per-user mailbox addresses on the freehire receiving
// domain (<handle>@<domain>). It is pure — allocation (picking the first free
// suffix against the store) lives in the handler service.
package mailbox

import (
	"strconv"
	"strings"
)

// fallbackHandle is used when an email's local-part sanitizes to nothing.
const fallbackHandle = "user"

// Handle derives the base handle from an email's local-part: everything before
// '@', lowercased, keeping only [a-z0-9.-] and dropping any other character. An
// empty result falls back to a fixed handle so allocation always has a base.
func Handle(email string) string {
	local := email
	if at := strings.IndexByte(local, '@'); at >= 0 {
		local = local[:at]
	}
	local = strings.ToLower(local)

	var b strings.Builder
	for _, r := range local {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return fallbackHandle
	}
	return b.String()
}

// reservedHandles are local-parts no user may hold on the receiving domain, because
// something other than a person is expected to answer them.
//
// Two reasons, both real. The RFC 2142 role names (and the operational ones around them)
// are where mail *about* the service arrives — an abuse report delivered into a stranger's
// job inbox is a report nobody reads. And the CA/Browser Forum's domain validation lets a
// certificate be issued to whoever answers a "constructed email address" (admin,
// administrator, webmaster, hostmaster, postmaster) at the domain, so handing one out
// hands out the ability to obtain a certificate for the receiving domain.
//
// Matching is on the exact local-part: a suffixed form like "postmaster-2" is not an
// operational address and nothing stops a user from holding it.
var reservedHandles = map[string]struct{}{
	// CA/Browser Forum constructed email addresses — certificate issuance.
	"admin": {}, "administrator": {}, "webmaster": {}, "hostmaster": {}, "postmaster": {},
	// RFC 2142 role addresses.
	"abuse": {}, "noc": {}, "security": {}, "usenet": {}, "news": {}, "www": {},
	"uucp": {}, "ftp": {}, "marketing": {}, "sales": {}, "support": {}, "info": {},
	// Bounce and automated-sender names: mail addressed to these is machine traffic, and
	// a human inbox answering them invites loops.
	"mailer-daemon": {}, "noreply": {}, "no-reply": {}, "bounce": {}, "bounces": {},
	// Domain-control and mail-authentication reporting.
	"dmarc": {}, "dkim": {}, "spf": {}, "root": {}, "ssl-admin": {}, "ssladmin": {},
}

// isReserved reports whether a candidate handle is one nobody may be allocated.
func isReserved(handle string) bool {
	_, ok := reservedHandles[handle]
	return ok
}

// Candidate returns the nth handle for a base: the base itself for n<=1, then
// "base-2", "base-3", … so a collision gets the smallest free suffix.
func Candidate(base string, n int) string {
	if n <= 1 {
		return base
	}
	return base + "-" + strconv.Itoa(n)
}

// Address composes the nth full address for a base handle on the given domain.
func Address(base string, n int, domain string) string {
	return Candidate(base, n) + "@" + domain
}
