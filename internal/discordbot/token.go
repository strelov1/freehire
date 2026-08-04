package discordbot

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
var ErrInvalidToken = errors.New("discordbot: invalid or expired link token")

// DiscordLinkTTL bounds how long a /link deep-link token is valid — long
// enough for a user to copy it from the web app into Discord, short enough
// that a leaked token is not useful for long.
const DiscordLinkTTL = 10 * time.Minute

// linkTokenPurpose is mixed into the MAC for domain separation, so a token
// signed for another purpose with the same secret cannot pass here.
const linkTokenPurpose = "discord-link"

// linkTokenMACLen truncates the HMAC to 16 bytes (128 bits) — ample for a
// short-lived token — to keep the encoded token small.
const linkTokenMACLen = 16

// linkTokenPayloadLen is the fixed payload: 8-byte user id + 8-byte expiry unix.
const linkTokenPayloadLen = 16

// DiscordLinkTokens mints and verifies the short, stateless token a user
// carries from the web app into the bot's /link slash command. Same shape as
// telegramnotify.LinkTokens: a base64url(payload‖MAC) blob, not a JWT — no
// server-side store needed, and it stays short enough to paste as a command
// argument.
type DiscordLinkTokens struct {
	secret []byte
	ttl    time.Duration
}

// NewDiscordLinkTokens returns a DiscordLinkTokens signing with secret (reuse
// JWT_SECRET) and expiring each token after ttl.
func NewDiscordLinkTokens(secret string, ttl time.Duration) *DiscordLinkTokens {
	return &DiscordLinkTokens{secret: []byte(secret), ttl: ttl}
}

// Issue returns a /link token for userID, expiring after the configured TTL.
func (l *DiscordLinkTokens) Issue(userID int64) (string, error) {
	payload := make([]byte, linkTokenPayloadLen)
	binary.BigEndian.PutUint64(payload[0:8], uint64(userID))
	binary.BigEndian.PutUint64(payload[8:16], uint64(time.Now().Add(l.ttl).Unix()))
	token := append(payload, l.mac(payload)...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

// Parse verifies a token's signature and expiry and returns its user id.
func (l *DiscordLinkTokens) Parse(token string) (int64, error) {
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
func (l *DiscordLinkTokens) mac(payload []byte) []byte {
	h := hmac.New(sha256.New, l.secret)
	h.Write([]byte(linkTokenPurpose))
	h.Write(payload)
	return h.Sum(nil)[:linkTokenMACLen]
}
