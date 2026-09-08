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

// ErrInvalidToken is the single sentinel every refusal wraps. The HTTP layer renders
// one generic failure for all of them: distinguishing "bad signature" from "no such
// account" in a response would tell an unauthenticated caller which user ids exist.
var ErrInvalidToken = errors.New("emailprefs: invalid token")

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

// silenceable reports whether a token may name g. Essential mail carries no
// unsubscribe affordance, so a token for it would link to a switch that does not
// exist.
func silenceable(g Group) bool {
	switch g {
	case GroupAlerts, GroupActivity, GroupNews:
		return true
	}
	return false
}

// Mint returns the token for one user and one group. The group is carried in the
// token, rather than the URL naming a generic "unsubscribe", because RFC 8058
// one-click acts without a confirmation screen: the target has to already know which
// mail it was reached from, or a tap on a campaign would silence job alerts too.
//
// Every part of the result is URL-safe, so it needs no escaping as a query value.
func (s *Signer) Mint(userID int64, g Group) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("emailprefs: mint for user id %d: %w", userID, ErrInvalidToken)
	}
	if !silenceable(g) {
		return "", fmt.Errorf("emailprefs: mint for group %q: %w", g, ErrInvalidToken)
	}
	payload := strconv.FormatInt(userID, 10) + "." + string(g)
	return payload + "." + s.sign(payload), nil
}

// Parse verifies a token and returns the user and group it names.
func (s *Signer) Parse(token string) (int64, Group, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, "", ErrInvalidToken
	}
	id, group, sig := parts[0], parts[1], parts[2]
	// The structural checks come first so a malformed token never reaches the
	// comparison, and so the comparison itself is the only thing that varies with
	// the secret.
	if !silenceable(Group(group)) {
		return 0, "", ErrInvalidToken
	}
	userID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || userID <= 0 {
		return 0, "", ErrInvalidToken
	}
	if !hmac.Equal([]byte(sig), []byte(s.sign(id+"."+group))) {
		return 0, "", ErrInvalidToken
	}
	return userID, Group(group), nil
}

// sign returns the base64url signature over payload.
func (s *Signer) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
