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
