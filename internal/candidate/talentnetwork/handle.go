package talentnetwork

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/classify"
)

// The catalogue handle: the one public identifier of a member's card, at
// /talent/<handle>. Minted the first time a candidate joins and never recomputed —
// a URL that moves is a URL somebody has already shared.
//
// It is deliberately NOT the account's username. internal/identity/username derives
// that from the email's local part, so for most accounts it is the person's own name,
// and the hosted mailbox adopts the same string as a live address. Either one in the
// URL of a page that exists to withhold the name would undo the feature in the
// address bar.
//
// The shape is validated here rather than by username.Valid for the same reason. That
// function owns the mailbox handle's rules; borrowing it would tie this URL's shape to
// a vocabulary that has no reason to move in step with it, and the drift would be
// silent in both directions.

const (
	// neutralBase is the handle base for a title the category dictionary does not
	// resolve. classify never guesses, and a candidate whose title it does not know is
	// not a candidate who should go without a URL.
	neutralBase = "candidate"

	// suffixLen is how many characters carry the uniqueness. Four over a 32-character
	// alphabet is ~1M values per base — a collision is possible and is simply re-minted
	// against the store, the same way internal/identity/accounts allocates a username.
	suffixLen = 4

	// maxHandleLen bounds what the route will look up. The longest category is well
	// under this; the limit exists so a crafted path is rejected before it reaches a
	// query, not to constrain anything we mint.
	maxHandleLen = 40
)

// suffixAlphabet has exactly 32 characters, which is the point: 32 divides 256, so a
// random byte maps to a character with no modulo bias and no rejection loop. 'l', 'o',
// '0' and '1' are left out — a handle gets read aloud and typed from a screenshot.
const suffixAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// handlePattern is the shape ValidHandle accepts: lowercase alphanumerics in
// hyphen-separated groups, at least two of them. The second group is the suffix, so a
// bare category can never be mistaken for somebody's handle.
var handlePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

// HandleBase returns the handle's readable part for a candidate whose most recent role
// carries the given title: the category the title resolves to, with the vocabulary's
// underscores written as the hyphens a URL wants, or the neutral base when nothing
// resolves.
//
// The SENIORITY is deliberately absent. A handle is frozen at mint, and the grade is
// the part of a title most likely to change — baking it in would guarantee that every
// member who gets promoted carries a URL contradicting their own card.
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
