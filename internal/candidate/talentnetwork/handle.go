package talentnetwork

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/classify"
)

// The catalogue handle: the one public identifier of a member's card, at /talent/<handle>.
// Minted when a candidate first joins and never recomputed, since a moved URL breaks shares.
//
// It is deliberately NOT the account's username (internal/identity/username), which is
// derived from the email local part, usually the person's name, and doubles as a mailbox
// address; either in the URL would defeat a page that withholds the name. For the same
// reason the shape is validated here, not by username.Valid.

const (
	// neutralBase is the handle base for a title the category dictionary does not resolve;
	// such a candidate still gets a URL.
	neutralBase = "candidate"

	// suffixLen is how many characters carry the uniqueness (~1M values per base); a
	// collision is re-minted against the store, like internal/identity/accounts usernames.
	suffixLen = 4

	// maxHandleLen bounds what the route will look up, so a crafted path is rejected
	// before it reaches a query.
	maxHandleLen = 40
)

// suffixAlphabet has exactly 32 characters, which is the point: 32 divides 256, so a
// random byte maps to a character with no modulo bias and no rejection loop. 'l', 'o',
// '0' and '1' are left out — a handle gets read aloud and typed from a screenshot.
const suffixAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// handlePattern is the shape ValidHandle accepts: lowercase alphanumerics in at least two
// hyphen-separated groups, so a bare category is never taken for a handle.
var handlePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

// HandleBase returns the handle's readable part for a most-recent role title: its
// category with underscores as hyphens, or the neutral base when nothing resolves.
//
// Seniority is deliberately absent: the handle is frozen at mint and the grade is what
// changes, so a promoted member would carry a URL contradicting their card.
func HandleBase(title string) string {
	category := classify.Parse(title).Category
	if category == "" {
		return neutralBase
	}
	return strings.ReplaceAll(category, "_", "-")
}

// MintHandle returns a fresh handle for the given most-recent role title. Two calls with
// the same title return different handles; uniqueness across accounts is the database's
// index, and a collision is re-minted by the caller.
func MintHandle(title string) (string, error) {
	suffix, err := randomSuffix()
	if err != nil {
		return "", err
	}
	return HandleBase(title) + "-" + suffix, nil
}

// randomSuffix returns suffixLen characters drawn uniformly from suffixAlphabet.
func randomSuffix() (string, error) {
	buf := make([]byte, suffixLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("talentnetwork: mint handle suffix: %w", err)
	}
	out := make([]byte, suffixLen)
	for i, b := range buf {
		out[i] = suffixAlphabet[int(b)%len(suffixAlphabet)]
	}
	return string(out), nil
}

// ValidHandle reports whether s could be a handle this package mints. The public route
// asks before it queries, so a crafted path is refused without touching the database.
func ValidHandle(s string) bool {
	if s == "" || len(s) > maxHandleLen {
		return false
	}
	return handlePattern.MatchString(s)
}
