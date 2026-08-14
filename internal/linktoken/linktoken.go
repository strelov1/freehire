// Package linktoken implements the short, stateless deep-link token shared by
// every "carry a user id from the web app into a chat bot" flow (Telegram,
// Discord, ...): a base64url(payload‖truncated-HMAC-SHA256) blob, not a JWT,
// so it survives being pasted as a deep-link start param or slash-command
// argument. Each caller wraps Tokens with its own purpose tag (for domain
// separation) and its own sentinel error, so this package carries only the
// encode/sign/verify logic — not the public API either caller exposes.
package linktoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"time"
)

// MACLen truncates the HMAC to 16 bytes (128 bits) — ample for a short-lived
// token — to keep the encoded token small.
const MACLen = 16

// PayloadLen is the fixed payload: 8-byte user id + 8-byte expiry unix.
const PayloadLen = 16

// Tokens mints and verifies a purpose-scoped link token.
type Tokens struct {
	secret  []byte
	purpose string
	ttl     time.Duration
}

// New returns Tokens signing with secret (reuse JWT_SECRET), scoped to
// purpose (mixed into the MAC so a token signed for another purpose with the
// same secret cannot pass verification here), expiring each token after ttl.
func New(secret, purpose string, ttl time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), purpose: purpose, ttl: ttl}
}

// Issue returns a token for userID, expiring after the configured TTL.
func (t *Tokens) Issue(userID int64) string {
	payload := make([]byte, PayloadLen)
	binary.BigEndian.PutUint64(payload[0:8], uint64(userID))
	binary.BigEndian.PutUint64(payload[8:16], uint64(time.Now().Add(t.ttl).Unix()))
	token := append(payload, t.mac(payload)...)
	return base64.RawURLEncoding.EncodeToString(token)
}

// Parse verifies a token's signature and expiry and returns its user id. ok
// is false for a malformed, mis-signed, or expired token.
func (t *Tokens) Parse(token string) (userID int64, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != PayloadLen+MACLen {
		return 0, false
	}
	payload, mac := raw[:PayloadLen], raw[PayloadLen:]
	if !hmac.Equal(mac, t.mac(payload)) {
		return 0, false
	}
	if exp := int64(binary.BigEndian.Uint64(payload[8:16])); time.Now().Unix() > exp {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(payload[0:8])), true
}

// mac is the truncated HMAC-SHA256 over the purpose tag and payload.
func (t *Tokens) mac(payload []byte) []byte {
	h := hmac.New(sha256.New, t.secret)
	h.Write([]byte(t.purpose))
	h.Write(payload)
	return h.Sum(nil)[:MACLen]
}
