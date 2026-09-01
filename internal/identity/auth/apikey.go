package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// apiKeyPrefix marks every freehire API key so a leaked secret is recognizable (to
// the user and to secret scanners) and never confused with a session token.
const apiKeyPrefix = "fhk_"

// apiKeyDisplayLen is how much of the token is kept as a non-secret display prefix
// (token_prefix): enough to tell keys apart in a list, far too little to guess the
// rest of a 256-bit secret.
const apiKeyDisplayLen = 12

// GenerateAPIKey mints a new opaque API key. It returns the plaintext token (shown
// to the user exactly once), its SHA-256 hash (the only thing persisted, and the
// per-request authentication lookup key), and a short non-secret display prefix.
func GenerateAPIKey() (token, hash, prefix string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", "", err
	}
	token = apiKeyPrefix + base64.RawURLEncoding.EncodeToString(b[:])
	return token, HashAPIKey(token), token[:apiKeyDisplayLen], nil
}

// HashAPIKey returns the hex-encoded SHA-256 of an API key. The token is
// high-entropy random, so a single SHA-256 is sufficient and keeps the lookup an
// indexed, constant-work probe — unlike a salted password hash, which a low-entropy
// password needs but which would forbid an indexed lookup.
//
// CodeQL reads this as password hashing (go/weak-sensitive-data-hashing, high) and
// the finding is dismissed, not silenced: nothing but a 32-byte crypto/rand token
// reaches here — GenerateAPIKey above, or the Bearer header in middleware.go — and
// passwords take an entirely separate path through bcrypt in password.go. Re-check
// that the two paths are still separate before dismissing it again; the rule is
// right about what it looks for, only wrong about what this argument is.
func HashAPIKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
