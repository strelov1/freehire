package auth

import (
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// PresentedBearer returns the Bearer credential a request carries, or "" when it carries none.
// It authenticates nothing: it only names the token, for a caller that resolves it on its own
// terms — the public-read rate limiter, which tells an API key (WellFormedAPIKey) from an MCP
// or CLI access token by shape and must not pay a lookup for anything else, a Bearer session
// JWT (the extension's transport) included.
func PresentedBearer(c *fiber.Ctx) string {
	return bearerToken(c)
}

// apiKeyLen is the length of every key GenerateAPIKey mints: the prefix and 32 random bytes
// in unpadded base64url.
var apiKeyLen = len(apiKeyPrefix) + base64.RawURLEncoding.EncodedLen(32)

// WellFormedAPIKey reports whether a token has the shape of a key we mint. Cheap and local,
// so a caller can refuse a made-up token before spending a database read on it.
func WellFormedAPIKey(token string) bool {
	if len(token) != apiKeyLen || !strings.HasPrefix(token, apiKeyPrefix) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token[len(apiKeyPrefix):])
	return err == nil
}
