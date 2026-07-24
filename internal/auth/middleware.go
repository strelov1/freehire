package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// LocalsUserID is the c.Locals key under which RequireAuth and RequireAuthOrKey
// store the authenticated user id. Handlers read it via UserID; it is exported
// because a websocket handler is handed a Conn rather than a Ctx and has to read
// the inherited local by key.
const LocalsUserID = "auth.userID"

// localsViaAPIKey is set true by RequireAuthOrKey when the request authenticated with an
// API key rather than the session cookie. Handlers read it via ViaAPIKey to give
// programmatic callers (e.g. the CV-tailoring agent) a narrower view than the owner's own
// browser session.
const localsViaAPIKey = "auth.viaAPIKey"

// ViaAPIKey reports whether the request authenticated via an API key (Bearer) rather than
// the session cookie. False for cookie auth and for requests that did not pass through
// RequireAuthOrKey.
func ViaAPIKey(c *fiber.Ctx) bool {
	via, _ := c.Locals(localsViaAPIKey).(bool)
	return via
}

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
		c.Locals(LocalsUserID, id)
		return c.Next()
	}
}

// UserID returns the authenticated user id stored by RequireAuth or
// RequireAuthOrKey. The second result is false when the request did not pass
// through either middleware.
func UserID(c *fiber.Ctx) (int64, bool) {
	id, ok := c.Locals(LocalsUserID).(int64)
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
// It responds 401 when neither credential resolves. A key-lookup failure that is
// not "no such key" (pgx.ErrNoRows) is a real error and is returned — it must not
// be masked as a 401.
func RequireAuthOrKey(iss *Issuer, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, err := iss.Parse(token); err == nil {
				c.Locals(LocalsUserID, id)
				return c.Next()
			}
		}
		id, viaKey, ok, err := resolveBearer(c, iss, keys)
		if err != nil {
			// A lookup failure (DB down, etc.) is not "unauthenticated":
			// surface it as a 500 rather than masking it as a 401.
			return err
		}
		if ok {
			c.Locals(LocalsUserID, id)
			if viaKey {
				c.Locals(localsViaAPIKey, true)
			}
			return c.Next()
		}
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
}

// OptionalAuth returns middleware that attaches the caller's user id when a valid
// session cookie or API key is present, and otherwise passes through anonymously.
// An absent, expired, or unknown credential simply leaves no user id in locals, so
// a public read still succeeds. Used on the job/company detail reads to overlay the
// caller's own vote without gating the page behind sign-in. A key-lookup failure
// that is not "no such key" (pgx.ErrNoRows) is a real error and is returned — it
// must not be silently degraded to anonymous.
func OptionalAuth(iss *Issuer, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, err := iss.Parse(token); err == nil {
				c.Locals(LocalsUserID, id)
				return c.Next()
			}
		}
		id, viaKey, ok, err := resolveBearer(c, iss, keys)
		if err != nil {
			// A lookup outage is a real error, not "anonymous": surface it.
			return err
		}
		if ok {
			c.Locals(LocalsUserID, id)
			if viaKey {
				c.Locals(localsViaAPIKey, true)
			}
		}
		return c.Next()
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

// resolveBearer authenticates an `Authorization: Bearer <token>` credential,
// accepting EITHER a session JWT or an API key. The browser extension presents
// its session JWT here (it has no cross-origin cookie), so a JWT bearer is a full
// session — viaKey is false; only an actual API key sets viaKey. Returns ok=false
// when there is no bearer or neither interpretation resolves. An API-key lookup
// failure that is not "no such key" (pgx.ErrNoRows) is a real error and is
// returned — it must not be silently masked as an unauthenticated request.
func resolveBearer(c *fiber.Ctx, iss *Issuer, keys APIKeyAuthenticator) (id int64, viaKey bool, ok bool, err error) {
	tok := bearerToken(c)
	if tok == "" {
		return 0, false, false, nil
	}
	if id, err := iss.Parse(tok); err == nil {
		return id, false, true, nil
	}
	id, err = keys.AuthenticateAPIKey(c.Context(), HashAPIKey(tok))
	switch {
	case err == nil:
		return id, true, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		// No live key matches — no credential resolved.
		return 0, false, false, nil
	default:
		// A lookup failure (DB down, etc.) is real: surface it to the caller.
		return 0, false, false, err
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
