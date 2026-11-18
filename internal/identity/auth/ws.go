package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// WSSubprotocolMarker is the first requested WebSocket subprotocol, with the session JWT
// second. Browsers cannot set headers on `new WebSocket`, so this is the only place a
// cross-origin extension can put a credential. The server echoes the marker alone.
const WSSubprotocolMarker = "freehire-jwt"

// SubprotocolToken extracts the credential from a "freehire-jwt, <token>"
// `Sec-WebSocket-Protocol` header, or "" for anything else.
func SubprotocolToken(header string) string {
	parts := strings.Split(header, ",")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != WSSubprotocolMarker {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// RequireAuthWS authenticates a WebSocket handshake and stores the user id in the same
// locals as RequireAuth, so the upgraded connection inherits it.
//
// A session JWT or an API key works from either carrier: the subprotocol (extension, no
// headers) or `Authorization: Bearer` (long-running harnesses holding a revocable key).
// Rejects with 401 when neither resolves.
func RequireAuthWS(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := bearerToken(c)
		if token == "" {
			token = SubprotocolToken(c.Get("Sec-WebSocket-Protocol"))
		}
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		// Full-scope only: driving a browser as the user is outside a CV credential's remit.
		b, ok, err := resolveCredential(c, iss, versions, keys, token)
		if err != nil {
			// A DB outage is a 503.
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		if b.viaKey && b.scope != ScopeFull {
			return fiber.NewError(fiber.StatusForbidden, "api key scope is insufficient for this endpoint")
		}
		c.Locals(LocalsUserID, b.userID)
		if b.viaKey {
			c.Locals(localsViaAPIKey, true)
			c.Locals(localsKeyScope, b.scope)
		}
		return c.Next()
	}
}
