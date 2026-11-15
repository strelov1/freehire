// Package emailprefs mints and verifies the token that lets someone holding one of
// our mails change what we send them without signing in.
//
// The token is stateless: nothing is stored and nothing expires, by design. A stored
// token would add a row per recipient per mail plus a retention sweep, and a way for
// the unsubscribe link to fail (row pruned or missing) in the one flow that must
// never fail. CAN-SPAM requires the opt-out to work for at least 30 days, and
// unsubscribing from an old archived mail is a preference we want, so expiry would
// be a bug.
//
// Rotating the signing secret is therefore the only revocation path, and it revokes
// every outstanding link at once.
package emailprefs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Group is the family of mail one switch governs. A mail belongs to exactly one,
// and the group is what decides which switch silences it.
type Group string

// The four groups. Three are silenceable and may be named by a token; the fourth
// cannot be turned off and therefore has no link to mint.
const (
	// GroupAlerts is saved-search digests.
	GroupAlerts Group = "alerts"
	// GroupActivity is mail about the recipient's own doings: saved-job reminders,
	// lifecycle nudges, reports.
	GroupActivity Group = "activity"
	// GroupNews is one-off campaigns, the onboarding sequence, and referral pings.
	GroupNews Group = "news"
	// GroupEssential is address verification and password reset. It is declared so
	// a mail can be labelled with it, never so a token can name it.
	GroupEssential Group = "essential"
)

// ErrInvalidToken is the single sentinel every Parse refusal wraps. The HTTP layer
// renders one generic failure for all of them, since telling "bad signature" from
// "no such account" would reveal which user ids exist. The wrapped cause stays out
// of the response and lets a log tell a secret rotation from junk traffic.
var ErrInvalidToken = errors.New("emailprefs: invalid token")

// ErrCannotMint is what Mint refuses with. Deliberately NOT ErrInvalidToken: a mint
// failure is our own bug, while ErrInvalidToken is a stranger's junk. A caller
// matching one sentinel must not catch both.
var ErrCannotMint = errors.New("emailprefs: cannot mint a token")

// keySalt separates this key from other uses of the same secret; without it an
// unsubscribe token and a session token would verify as each other.
const keySalt = "email-prefs-v1"

// Signer mints and verifies tokens. It holds only the derived key, so share one.
type Signer struct {
	key []byte
}

// NewSigner derives the signing key from secret, the same one the session issuer
// uses; keySalt keeps the two keys apart.
func NewSigner(secret string) *Signer {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(keySalt))
	return &Signer{key: mac.Sum(nil)}
}

// silenceableGroups is the vocabulary, in preference-page order: mail someone asked
// for, mail about their own doings, mail we send on our own initiative.
//
// It is the one copy; senders, the turn-everything-off endpoint and the preference
// page all read it from here, so a new group reaches all of them.
var silenceableGroups = [...]Group{GroupAlerts, GroupActivity, GroupNews}

// SilenceableGroups returns the groups a token may name and a preference page must
// offer, in display order. It returns a fresh copy each call, so callers cannot
// alias the vocabulary.
func SilenceableGroups() []Group { return slices.Clone(silenceableGroups[:]) }

// Silenceable reports whether g is a group somebody may turn off. Essential mail has
// no switch to link to; an unknown group is refused the same way.
func Silenceable(g Group) bool { return slices.Contains(silenceableGroups[:], g) }

// Mint returns the token for one user and one group. The group rides in the token
// because RFC 8058 one-click acts without a confirmation screen, so the target must
// already know which mail it came from.
//
// Every part of the result is URL-safe, so it needs no escaping as a query value.
func (s *Signer) Mint(userID int64, g Group) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("%w: user id %d", ErrCannotMint, userID)
	}
	if !Silenceable(g) {
		return "", fmt.Errorf("%w: group %q cannot be turned off", ErrCannotMint, g)
	}
	payload := payloadFor(userID, g)
	return payload + "." + s.sign(payload), nil
}

// Parse verifies a token and returns the user and group it names.
//
// A successful parse proves only that we minted this token. It does NOT prove the
// account still exists — there is no database here — so a caller must still look the
// user up and refuse a token naming an account that has since been deleted, with the
// same generic failure it gives a forged one.
func (s *Signer) Parse(token string) (int64, Group, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, "", fmt.Errorf("%w: want 3 dot-separated parts, got %d", ErrInvalidToken, len(parts))
	}
	id, group, sig := parts[0], parts[1], parts[2]
	// Structural checks first, so a malformed token never reaches the comparison.
	g := Group(group)
	if !Silenceable(g) {
		return 0, "", fmt.Errorf("%w: unknown or unsilenceable group %q", ErrInvalidToken, group)
	}
	userID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || userID <= 0 {
		return 0, "", fmt.Errorf("%w: user id %q", ErrInvalidToken, id)
	}
	// Re-sign from the PARSED id: ParseInt accepts "+42" and "0042", which Mint never
	// emits, so the canonical form guarantees those never verify.
	if !hmac.Equal([]byte(sig), []byte(s.sign(payloadFor(userID, g)))) {
		return 0, "", fmt.Errorf("%w: signature mismatch (a rotated secret looks like this)", ErrInvalidToken)
	}
	return userID, g, nil
}

// payloadFor is the signed string, built in one place so Mint and Parse cannot
// drift. The "." is unambiguous because an id is digits and a group name has no dot
// (TestNoGroupNameCarriesTheSeparator).
func payloadFor(userID int64, g Group) string {
	return strconv.FormatInt(userID, 10) + "." + string(g)
}

// sign returns the base64url signature over payload.
func (s *Signer) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
