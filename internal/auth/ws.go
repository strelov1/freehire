package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// WSSubprotocolMarker is the literal a browser client sends as its first
// requested WebSocket subprotocol, with the session JWT as the second. Browsers
// cannot set headers on `new WebSocket(url, protocols)`, so the subprotocol slot
// is the only place a cross-origin extension can put a credential — the same
// trick the Roy client already uses. The server echoes back the marker alone,
// never the token.
const WSSubprotocolMarker = "freehire-jwt"

// SubprotocolToken extracts the credential from a `Sec-WebSocket-Protocol`
// header of the form "freehire-jwt, <token>", returning "" for anything else so
// an unmarked or empty header simply fails to authenticate.
func SubprotocolToken(header string) string {
	parts := strings.Split(header, ",")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != WSSubprotocolMarker {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// RequireAuthWS returns middleware that authenticates a WebSocket handshake and
// stores the resolved user id in the same locals as RequireAuth, so the upgraded
// connection inherits it.
//
// Either credential works, from either carrier. The carrier is about what the
// client can do: a browser extension has no headers on `new WebSocket` and no
// cross-origin cookie, so it uses the subprotocol; anything else sends
// `Authorization: Bearer`. The credential is about what the client holds: the
// extension has a session JWT from the connect flow, while a long-running
// harness holds an API key (what the CLI uses) it can keep and revoke, rather
// than a short-lived token it has no way to re-mint.
//
// Rejects with 401 when neither interpretation resolves.
func RequireAuthWS(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := bearerToken(c)
		if token == "" {
			token = SubprotocolToken(c.Get("Sec-WebSocket-Protocol"))
		}
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		// Full-scope only, like every other non-CV surface: driving a browser as the
		// user is well outside what a CV-tailoring credential was minted for.
		b, ok, err := resolveCredential(c, iss, versions, keys, token)
		if err != nil {
			// A DB infrastructure outage is a 503 service unavailable, not a silent failure or raw error.
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
