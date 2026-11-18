package auth

import (
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// PresentedBearer returns the Bearer credential a request carries, or "". It authenticates
// nothing; the public-read rate limiter uses it to tell an API key from other tokens by
// shape without a lookup.
func PresentedBearer(c *fiber.Ctx) string {
	return bearerToken(c)
}

// apiKeyLen is the length of a minted key: prefix plus 32 random bytes in unpadded base64url.
var apiKeyLen = len(apiKeyPrefix) + base64.RawURLEncoding.EncodedLen(32)

// WellFormedAPIKey reports whether a token has the shape of a key we mint, so a made-up
// token is refused without a database read.
func WellFormedAPIKey(token string) bool {
	if len(token) != apiKeyLen || !strings.HasPrefix(token, apiKeyPrefix) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token[len(apiKeyPrefix):])
	return err == nil
}
