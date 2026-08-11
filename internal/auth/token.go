package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer mints and verifies stateless HS256 JWTs that carry the user id as the
// subject and the account's session generation as a private claim. One Issuer is
// built from the configured secret and TTL and shared across requests.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

// sessionClaims is the token payload: the registered claims plus the account's
// token version. The version is what makes a stateless token revocable — bumping
// the column strands every token minted under the old value.
//
// TokenVersion is a pointer so an absent claim is distinguishable from zero.
// Accounts start at version 1, so a token minted before versioning existed — which
// decodes to a missing claim — is rejected rather than silently matching.
type sessionClaims struct {
	jwt.RegisteredClaims
	TokenVersion *int32 `json:"tv,omitempty"`
}

// ErrNoTokenVersion reports a correctly signed token that predates session
// versioning. It is distinct from a signature or expiry failure so a caller can tell
// "revoked by the release" apart from "forged".
var ErrNoTokenVersion = errors.New("auth: token carries no version claim")

// ErrNoSessionID reports a valid legacy session minted before per-session JTIs
// were introduced. Such a token may remain usable for ordinary requests during
// rollout, but it cannot safely bind a recent-auth proof until it is rotated.
var ErrNoSessionID = errors.New("auth: token carries no session id")

// NewIssuer returns an Issuer signing with secret and stamping each token to
// expire after ttl.
func NewIssuer(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl}
}

// TTL is the token lifetime; the cookie helpers use it as the cookie's max-age
// so the browser and the token expire together.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue returns a signed token for userID stamped with the account's current
// tokenVersion, expiring after the Issuer's TTL.
func (i *Issuer) Issue(userID int64, tokenVersion int32) (string, error) {
	sessionIDBytes := make([]byte, 16)
	if _, err := rand.Read(sessionIDBytes); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	now := time.Now()
	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			ID:        base64.RawURLEncoding.EncodeToString(sessionIDBytes),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
		TokenVersion: &tokenVersion,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
}

func fingerprint(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// SessionFingerprint verifies token and returns a one-way binding for an exact,
// JTI-stamped session. Legacy no-JTI sessions must be rotated before this method
// will authorize a recent-auth proof.
func (i *Issuer) SessionFingerprint(token string) ([]byte, error) {
	claims, err := i.parseClaims(token)
	if err != nil {
		return nil, err
	}
	if claims.ID == "" {
		return nil, ErrNoSessionID
	}
	return fingerprint(token), nil
}

func (i *Issuer) parseClaims(token string) (*sessionClaims, error) {
	claims := &sessionClaims{}
	if _, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return i.secret, nil
	}); err != nil {
		return nil, err
	}
	return claims, nil
}

// Parse verifies a token's signature and expiry and returns its subject user id
// and version claim. It rejects any token not signed with HMAC (guarding against
// algorithm confusion), any whose subject is not a valid id, and any carrying no
// version claim.
func (i *Issuer) Parse(token string) (int64, int32, error) {
	claims, err := i.parseClaims(token)
	if err != nil {
		return 0, 0, err
	}

	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid token subject: %w", err)
	}
	if claims.TokenVersion == nil {
		return 0, 0, ErrNoTokenVersion
	}
	return id, *claims.TokenVersion, nil
}
