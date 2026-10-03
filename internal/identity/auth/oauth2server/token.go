package oauth2server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// accessTokenPrefix marks an MCP access token so a leaked secret is recognizable,
// the same way apiKeyPrefix ("fhk_") marks an API key — see auth.GenerateAPIKey.
const accessTokenPrefix = "fhm_"

// authorizationCodePrefix marks a one-time authorization code, distinct from an
// access token so the two can never be confused if one leaks into the wrong log line.
const authorizationCodePrefix = "fhc_"

// GenerateAccessToken mints a 256-bit opaque bearer token for a newly approved
// grant. Returns the plaintext (shown to the client exactly once, in the token
// endpoint's response body) and its SHA-256 hash (the only thing persisted).
func GenerateAccessToken() (token, hash string, err error) {
	return generateOpaque(accessTokenPrefix)
}

// GenerateAuthorizationCode mints the single-use code redirected back to the
// client after consent. Same shape as GenerateAccessToken, different prefix.
func GenerateAuthorizationCode() (code, hash string, err error) {
	return generateOpaque(authorizationCodePrefix)
}

func generateOpaque(prefix string) (plaintext, hash string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	plaintext = prefix + base64.RawURLEncoding.EncodeToString(b[:])
	return plaintext, HashToken(plaintext), nil
}

// HashToken returns the hex-encoded SHA-256 of a token or code. High-entropy
// crypto/rand input makes a single SHA-256 sufficient for an indexed lookup — the
// same reasoning auth.HashAPIKey documents for API keys; passwords stay on bcrypt
// in auth/password.go, an entirely separate path.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
