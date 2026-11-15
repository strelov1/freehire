package oauth2server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// accessTokenPrefix marks an MCP access token so a leak is recognizable, like apiKeyPrefix.
const accessTokenPrefix = "fhm_"

// authorizationCodePrefix marks a one-time code, distinct from an access token.
const authorizationCodePrefix = "fhc_"

// GenerateAccessToken mints a 256-bit opaque bearer token: the plaintext (shown once, in
// the token response) and its SHA-256 hash (the only thing persisted).
func GenerateAccessToken() (token, hash string, err error) {
	return generateOpaque(accessTokenPrefix)
}

// accessTokenLen is the length of a minted token: prefix plus 32 random bytes in base64url.
var accessTokenLen = len(accessTokenPrefix) + base64.RawURLEncoding.EncodedLen(32)

// WellFormedAccessToken reports whether a token has the shape we mint, so a made-up one is
// refused without a database read.
func WellFormedAccessToken(token string) bool {
	if len(token) != accessTokenLen || !strings.HasPrefix(token, accessTokenPrefix) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token[len(accessTokenPrefix):])
	return err == nil
}

// GenerateAuthorizationCode mints the single-use code redirected to the client after
// consent. Same shape as GenerateAccessToken, different prefix.
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

// HashToken returns the hex SHA-256 of a token or code. High-entropy input makes one
// SHA-256 sufficient for an indexed lookup (see auth.HashAPIKey); passwords stay on bcrypt.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
