package auth

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// localsUserID is the c.Locals key under which RequireAuth and RequireAuthOrKey
// store the authenticated user id. Handlers read it via UserID.
const localsUserID = "auth.userID"

// RequireAuth returns middleware that validates the auth cookie and stores the
// resolved user id in the request locals. It responds 401 on a missing,
// expired, or invalid token.
func RequireAuth(iss *Issuer) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies(CookieName)
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		id, err := iss.Parse(token)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired session")
		}
		c.Locals(localsUserID, id)
		return c.Next()
	}
}

// UserID returns the authenticated user id stored by RequireAuth or
// RequireAuthOrKey. The second result is false when the request did not pass
// through either middleware.
func UserID(c *fiber.Ctx) (int64, bool) {
	id, ok := c.Locals(localsUserID).(int64)
	return id, ok
}

// APIKeyAuthenticator resolves a presented API-key hash to the owning user id,
// returning an error when no live key matches. It is satisfied directly by
// *db.Queries (AuthenticateAPIKey), so this package needs no database import.
type APIKeyAuthenticator interface {
	AuthenticateAPIKey(ctx context.Context, tokenHash string) (int64, error)
}

// RequireAuthOrKey returns middleware that authenticates a request by either the
// session cookie (the existing JWT path) or an `Authorization: Bearer <key>` API
// key, storing the resolved user id in the same locals as RequireAuth — so every
// handler behind it works unchanged. The cookie is tried first, leaving the
// browser path identical; a missing or invalid cookie falls through to the key.
// It responds 401 when neither credential resolves.
func RequireAuthOrKey(iss *Issuer, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, err := iss.Parse(token); err == nil {
				c.Locals(localsUserID, id)
				return c.Next()
			}
		}
		if key := bearerToken(c); key != "" {
			if id, err := keys.AuthenticateAPIKey(c.Context(), HashAPIKey(key)); err == nil {
				c.Locals(localsUserID, id)
				return c.Next()
			}
		}
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
}

// RoleLoader resolves an authenticated user id to its current role. It is satisfied
// directly by *db.Queries (GetUserRole), so this package needs no database import.
type RoleLoader interface {
	GetUserRole(ctx context.Context, id int64) (string, error)
}

// RequireRole returns middleware that authorizes a request by the caller's role. It
// runs AFTER an authentication middleware (RequireAuth/RequireAuthOrKey) has stored the
// user id, reads that id, loads the current role from the database, and rejects unless
// it matches. The role is read fresh per request (not from the token) so a role change
// takes effect immediately. Failures fail closed: a missing user id or a role-load error
// (e.g. the token's user no longer exists) is a 401; a role that does not match is a 403.
func RequireRole(loader RoleLoader, role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := UserID(c)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		got, err := loader.GetUserRole(c.Context(), id)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		if got != role {
			return fiber.NewError(fiber.StatusForbidden, "forbidden")
		}
		return c.Next()
	}
}

// BetaLoader resolves an authenticated user id to its beta-tester membership. Like
// RoleLoader it returns a primitive so this package needs no database import; it is
// satisfied directly by *db.Queries (IsBetaTester).
type BetaLoader interface {
	IsBetaTester(ctx context.Context, id int64) (bool, error)
}

// RequireModeratorOrBeta authorizes a request when the caller is EITHER a moderator
// OR a beta tester — for restricted-rollout features that moderators administer and
// beta testers get early access to (e.g. the mail inbox). Same fail-closed discipline
// as RequireRole: no user id → 401; neither moderator nor beta → 403.
func RequireModeratorOrBeta(roles RoleLoader, beta BetaLoader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, ok := UserID(c)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		if role, err := roles.GetUserRole(c.Context(), id); err == nil && role == "moderator" {
			return c.Next()
		}
		isBeta, err := beta.IsBetaTester(c.Context(), id)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		if !isBeta {
			return fiber.NewError(fiber.StatusForbidden, "forbidden")
		}
		return c.Next()
	}
}

// bearerToken extracts the credential from an `Authorization: Bearer <token>`
// header, returning "" when the header is absent or not a Bearer scheme.
func bearerToken(c *fiber.Ctx) string {
	const prefix = "Bearer "
	h := c.Get(fiber.HeaderAuthorization)
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		// A Bearer credential carries no internal whitespace; trim any the client
		// or a proxy added around it so a valid key is not silently mismatched.
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
