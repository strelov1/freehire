// Package discordbot is the Discord-side counterpart to internal/telegramnotify:
// Ed25519 request verification, the two outbound Discord API calls this bot
// needs (editing a deferred interaction response, registering slash commands),
// the interaction JSON shapes, and the /link deep-link token.
package discordbot

import (
	"crypto/ed25519"
	"encoding/hex"
)

// VerifySignature checks a Discord interaction webhook request against the
// application's public key, per Discord's required PING verification: the
// signature covers the concatenation of the raw timestamp header and the raw
// request body, in that order. Any decode failure (bad hex, wrong key/sig
// length) is treated as an invalid signature rather than a panic or error.
func VerifySignature(publicKeyHex string, timestamp, body []byte, signatureHex string) bool {
	pubKey, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(signatureHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	msg := make([]byte, 0, len(timestamp)+len(body))
	msg = append(msg, timestamp...)
	msg = append(msg, body...)
	return ed25519.Verify(pubKey, msg, sig)
}
