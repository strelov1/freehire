package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// apiKeyPrefix marks freehire API keys so leaks are recognizable and never confused with
// a session token.
const apiKeyPrefix = "fhk_"

// apiKeyDisplayLen is the non-secret display prefix kept as token_prefix.
const apiKeyDisplayLen = 12

// GenerateAPIKey mints an API key: the plaintext token (shown once), its SHA-256 hash (the
// only thing persisted, and the lookup key), and a short display prefix.
func GenerateAPIKey() (token, hash, prefix string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", "", err
	}
	token = apiKeyPrefix + base64.RawURLEncoding.EncodeToString(b[:])
	return token, HashAPIKey(token), token[:apiKeyDisplayLen], nil
}

// HashAPIKey returns the hex SHA-256 of an API key. The token is high-entropy random, so
// one SHA-256 suffices and keeps the lookup indexed.
//
// CodeQL flags this as password hashing (go/weak-sensitive-data-hashing) and the finding
// is dismissed: only 32-byte crypto/rand tokens reach here, and passwords go through
// bcrypt in password.go. Re-check that the paths stay separate.
func HashAPIKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
