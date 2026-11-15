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

// Issuer mints and verifies stateless HS256 JWTs carrying the user id (subject) and the
// account's session generation (private claim).
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

// sessionClaims is the token payload. The token version makes a stateless token revocable:
// bumping the column strands older tokens. TokenVersion is a pointer so a missing claim
// (a pre-versioning token) is rejected instead of matching zero.
type sessionClaims struct {
	jwt.RegisteredClaims
	TokenVersion *int32 `json:"tv,omitempty"`
}

// ErrNoTokenVersion reports a correctly signed token that predates session versioning,
// distinct from a forged or expired one.
var ErrNoTokenVersion = errors.New("auth: token carries no version claim")

// ErrNoSession reports a request that passed through no session middleware.
var ErrNoSession = errors.New("auth: no session")

// ErrNoSessionID reports a legacy session without a JTI. It works for ordinary requests
// but cannot bind a recent-auth proof until rotated.
var ErrNoSessionID = errors.New("auth: token carries no session id")

// NewIssuer returns an Issuer signing with secret; tokens expire after ttl.
func NewIssuer(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl}
}

// TTL is the token lifetime, also used as the cookie max-age.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// ExpiresAt verifies the session token and reports its signed expiry for introspection.
func (i *Issuer) ExpiresAt(token string) (time.Time, error) {
	claims, err := i.parseClaims(token)
	if err != nil {
		return time.Time{}, err
	}
	if claims.ExpiresAt == nil {
		return time.Time{}, nil
	}
	return claims.ExpiresAt.Time, nil
}

// Issue returns a signed token for userID stamped with tokenVersion.
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

// Parse verifies signature and expiry and returns the user id and version claim. It
// rejects non-HMAC algorithms (algorithm confusion), invalid subjects and a missing version.
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
