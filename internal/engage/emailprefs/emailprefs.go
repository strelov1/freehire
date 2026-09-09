// Package emailprefs mints and verifies the token that lets someone holding one of
// our mails change what we send them without signing in.
//
// The token is stateless: nothing is stored and nothing expires. That is a decision,
// not a shortcut. A stored token would buy revocation and open-tracking, neither of
// which this flow wants, at the cost of a row per recipient per mail — thousands per
// campaign — plus a retention sweep, and it would introduce a way for the unsubscribe
// link to fail (row pruned, row missing) in the one flow that must never fail.
// CAN-SPAM additionally requires the opt-out to work for at least 30 days after a
// send, and a person unsubscribing from a mail found in an old archive is expressing
// exactly the preference we want to record, so an expiry would be a bug.
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
// renders one generic failure for all of them: distinguishing "bad signature" from
// "no such account" in a response would tell an unauthenticated caller which user
// ids exist. Each refusal wraps a cause as context, which stays out of the response
// and lets a log tell a secret rotation — the documented revocation path, and so the
// likeliest source of a sudden flood — apart from ordinary junk traffic.
var ErrInvalidToken = errors.New("emailprefs: invalid token")

// ErrCannotMint is what Mint refuses with. It is deliberately NOT ErrInvalidToken:
// a mint failure means our own code asked for a link it may not have, which is a bug
// worth paging someone over, while an ErrInvalidToken means a stranger sent junk,
// which is Tuesday. A caller matching one sentinel must not catch both.
var ErrCannotMint = errors.New("emailprefs: cannot mint a token")

// keySalt separates this key from every other use of the same secret. Without it an
// unsubscribe token and a session token would be signed by the same key, and the
// verifier for one would accept the other.
const keySalt = "email-prefs-v1"

// Signer mints and verifies tokens. Build it once and share it; it holds no state
// beyond the derived key.
type Signer struct {
	key []byte
}

// NewSigner derives the signing key from secret. The caller passes the same secret
// the session issuer uses — keySalt is what keeps the two keys apart.
func NewSigner(secret string) *Signer {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(keySalt))
	return &Signer{key: mac.Sum(nil)}
}

// silenceableGroups is the vocabulary, in the order a preference page reads: the
// mail someone asked for, then the mail about their own doings, then the mail we
// send on our own initiative.
//
// It is the one copy. Every consumer — the senders that mint a link, the endpoint
// that turns everything off, the page that renders a switch per group — reads it
// from here rather than writing {alerts, activity, news} again, because a
// hand-maintained list checked against another hand-maintained list proves
// consistency and not coverage. A fourth group added below reaches all of them.
var silenceableGroups = [...]Group{GroupAlerts, GroupActivity, GroupNews}

// SilenceableGroups returns the groups a token may name and a preference page must
// offer, in display order. It returns a fresh slice each call: the vocabulary is
// held in an array so that handing it out cannot alias it, and a caller that writes
// to what it was given edits its own copy rather than everyone's.
func SilenceableGroups() []Group { return slices.Clone(silenceableGroups[:]) }

// Silenceable reports whether g is a group somebody may turn off. Essential mail
// carries no unsubscribe affordance, so a token for it would link to a switch that
// does not exist; an unknown group is refused by the same answer.
func Silenceable(g Group) bool { return slices.Contains(silenceableGroups[:], g) }

// Mint returns the token for one user and one group. The group is carried in the
// token, rather than the URL naming a generic "unsubscribe", because RFC 8058
// one-click acts without a confirmation screen: the target has to already know which
// mail it was reached from, or a tap on a campaign would silence job alerts too.
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
	// The structural checks come first so a malformed token never reaches the
	// comparison, and so the comparison itself is the only thing that varies with
	// the secret.
	g := Group(group)
	if !Silenceable(g) {
		return 0, "", fmt.Errorf("%w: unknown or unsilenceable group %q", ErrInvalidToken, group)
	}
	userID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || userID <= 0 {
		return 0, "", fmt.Errorf("%w: user id %q", ErrInvalidToken, id)
	}
	// Re-signed from the PARSED user id, not the string as it arrived. ParseInt
	// accepts "+42" and "0042", which Mint never emits; signing the canonical form
	// means those never verify, rather than not verifying only because nothing
	// signs them today.
	if !hmac.Equal([]byte(sig), []byte(s.sign(payloadFor(userID, g)))) {
		return 0, "", fmt.Errorf("%w: signature mismatch (a rotated secret looks like this)", ErrInvalidToken)
	}
	return userID, g, nil
}

// payloadFor is the signed string, built in exactly one place so Mint and Parse
// cannot drift. The "." is unambiguous because an id is decimal digits and a group
// name carries no dot — TestNoGroupNameCarriesTheSeparator holds the second half.
func payloadFor(userID int64, g Group) string {
	return strconv.FormatInt(userID, 10) + "." + string(g)
}

// sign returns the base64url signature over payload.
func (s *Signer) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
