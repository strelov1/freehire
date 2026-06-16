// Package telegramnotify is the outbound Telegram channel for filter-subscription
// notifications — the sibling of the inbound internal/telegram crawl. It mints the
// signed deep-link token that links a user's chat, talks to the Bot API
// (sendMessage/setWebhook), parses inbound webhook updates, and implements
// notify.Notifier by rendering a digest into a Telegram message.
package telegramnotify

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// linkPurpose scopes the link token so a session JWT (or any other token) cannot
// be replayed as a link token, and vice versa.
const linkPurpose = "tg-link"

// ErrWrongPurpose is returned when a structurally valid token is not a link token.
var ErrWrongPurpose = errors.New("telegramnotify: token is not a link token")

// LinkTokens mints and verifies short-lived signed tokens a user carries into the
// bot via the t.me deep link. The token identifies the user (subject) and is
// purpose-scoped; it is stateless (no server-side token store) and expires.
type LinkTokens struct {
	secret []byte
	ttl    time.Duration
}

// NewLinkTokens returns a LinkTokens signing with secret (reuse JWT_SECRET) and
// expiring each token after ttl (a short window, e.g. 10 minutes).
func NewLinkTokens(secret string, ttl time.Duration) *LinkTokens {
	return &LinkTokens{secret: []byte(secret), ttl: ttl}
}

type linkClaims struct {
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

// Issue returns a signed link token for userID, expiring after the configured TTL.
func (l *LinkTokens) Issue(userID int64) (string, error) {
	now := time.Now()
	claims := linkClaims{
		Purpose: linkPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(l.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(l.secret)
}

// Parse verifies a link token's signature, expiry, and purpose, returning its
// subject user id. It rejects any token not signed with HMAC (algorithm-confusion
// guard) and any whose purpose is not the link purpose.
func (l *LinkTokens) Parse(token string) (int64, error) {
	claims := &linkClaims{}
	if _, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return l.secret, nil
	}); err != nil {
		return 0, err
	}
	if claims.Purpose != linkPurpose {
		return 0, ErrWrongPurpose
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid token subject: %w", err)
	}
	return id, nil
}
