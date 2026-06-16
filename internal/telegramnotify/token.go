// Package telegramnotify is the outbound Telegram channel for filter-subscription
// notifications — the sibling of the inbound internal/telegram crawl. It mints the
// deep-link token that links a user's chat, talks to the Bot API
// (sendMessage/setWebhook), parses inbound webhook updates, and implements
// notify.Notifier by rendering a digest into a Telegram message.
package telegramnotify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

// ErrInvalidToken is returned for a link token that is malformed, has a bad
// signature, or has expired.
var ErrInvalidToken = errors.New("telegramnotify: invalid or expired link token")

// linkTokenPurpose is mixed into the MAC for domain separation, so a token signed
// for another purpose with the same secret cannot pass here.
const linkTokenPurpose = "tg-link"

// linkTokenMACLen truncates the HMAC to 16 bytes (128 bits) — ample for a
// short-lived token — to keep the encoded token small.
const linkTokenMACLen = 16

// linkTokenPayloadLen is the fixed payload: 8-byte user id + 8-byte expiry unix.
const linkTokenPayloadLen = 16

// LinkTokens mints and verifies the short, stateless token a user carries into the
// bot via the t.me deep link. The encoding is deliberately NOT a JWT: Telegram's
// deep-link `start` parameter allows only 1–64 chars from [A-Za-z0-9_-], which a
// JWT (dotted, ~200 chars) violates. This token is a base64url(payload‖MAC) blob
// of ~43 chars using exactly that alphabet, so it survives the deep link intact.
type LinkTokens struct {
	secret []byte
	ttl    time.Duration
}

// NewLinkTokens returns a LinkTokens signing with secret (reuse JWT_SECRET) and
// expiring each token after ttl (a short window, e.g. 10 minutes).
func NewLinkTokens(secret string, ttl time.Duration) *LinkTokens {
	return &LinkTokens{secret: []byte(secret), ttl: ttl}
}

// Issue returns a deep-link token for userID, expiring after the configured TTL.
// The result is base64url (no padding) so it is safe as a Telegram start param.
func (l *LinkTokens) Issue(userID int64) (string, error) {
	payload := make([]byte, linkTokenPayloadLen)
	binary.BigEndian.PutUint64(payload[0:8], uint64(userID))
	binary.BigEndian.PutUint64(payload[8:16], uint64(time.Now().Add(l.ttl).Unix()))
	token := append(payload, l.mac(payload)...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

// Parse verifies a token's signature and expiry and returns its user id.
func (l *LinkTokens) Parse(token string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != linkTokenPayloadLen+linkTokenMACLen {
		return 0, ErrInvalidToken
	}
	payload, mac := raw[:linkTokenPayloadLen], raw[linkTokenPayloadLen:]
	if !hmac.Equal(mac, l.mac(payload)) {
		return 0, ErrInvalidToken
	}
	if exp := int64(binary.BigEndian.Uint64(payload[8:16])); time.Now().Unix() > exp {
		return 0, ErrInvalidToken
	}
	return int64(binary.BigEndian.Uint64(payload[0:8])), nil
}

// mac is the truncated HMAC-SHA256 over the purpose tag and payload.
func (l *LinkTokens) mac(payload []byte) []byte {
	h := hmac.New(sha256.New, l.secret)
	h.Write([]byte(linkTokenPurpose))
	h.Write(payload)
	return h.Sum(nil)[:linkTokenMACLen]
}
