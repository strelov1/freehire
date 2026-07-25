package auth

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
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

// localsKeyScope holds the authenticating key's scope. Absent for cookie sessions, which
// are unscoped by construction — a browser session is the owner themselves.
const localsKeyScope = "auth.keyScope"

// Scopes an API key can carry. ScopeFull is everything a key may do and is what a
// user-created key gets; ScopeCV is the CV surface a tailoring agent needs and nothing
// else, so a credential handed to an external agent cannot reach the rest of the account.
const (
	ScopeFull = "full"
	ScopeCV   = "cv"
)

// ViaAPIKey reports whether the request authenticated via an API key (Bearer) rather than
// the session cookie. False for cookie auth and for requests that did not pass through
// RequireAuthOrKey.
func ViaAPIKey(c *fiber.Ctx) bool {
	via, _ := c.Locals(localsViaAPIKey).(bool)
	return via
}

// KeyScope returns the authenticating key's scope, or "" when the caller is a cookie
// session (or did not pass through a key-accepting middleware).
func KeyScope(c *fiber.Ctx) string {
	scope, _ := c.Locals(localsKeyScope).(string)
	return scope
}

// TokenVersionLoader resolves an authenticated user id to its current session
// generation. Like RoleLoader it returns a primitive so this package needs no database
// import; it is satisfied directly by *db.Queries (GetUserTokenVersion).
type TokenVersionLoader interface {
	GetUserTokenVersion(ctx context.Context, id int64) (int32, error)
}

// RequireAuth returns middleware that validates the auth cookie and stores the
// resolved user id in the request locals. It responds 401 on a missing,
// expired, invalid, or revoked token.
func RequireAuth(iss *Issuer, versions TokenVersionLoader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies(CookieName)
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		id, ok := resolveSession(c, iss, versions, token)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired session")
		}
		c.Locals(LocalsUserID, id)
		return c.Next()
	}
}

// resolveSession validates a cookie token and confirms it was not revoked, returning the
// user id it authenticates. A version-load failure (deleted account, database trouble)
// fails closed, matching RequireRole: an unverifiable session is not a session.
func resolveSession(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, token string) (int64, bool) {
	id, version, err := iss.Parse(token)
	if err != nil {
		return 0, false
	}
	current, err := versions.GetUserTokenVersion(c.Context(), id)
	if err != nil || current != version {
		return 0, false
	}
	return id, true
}

// UserID returns the authenticated user id stored by RequireAuth or
// RequireAuthOrKey. The second result is false when the request did not pass
// through either middleware.
func UserID(c *fiber.Ctx) (int64, bool) {
	id, ok := c.Locals(LocalsUserID).(int64)
	return id, ok
}

// APIKeyIdentity is what a live API key resolves to: its owner and the scope it was
// minted with. It is a plain struct so this package needs no database import.
type APIKeyIdentity struct {
	UserID int64
	Scope  string
}

// APIKeyAuthenticator resolves a presented API-key hash to its owner and scope,
// returning an error when no live key matches.
type APIKeyAuthenticator interface {
	AuthenticateAPIKey(ctx context.Context, tokenHash string) (APIKeyIdentity, error)
}

// RequireAuthOrKey returns middleware that authenticates a request by either the
// session cookie (the existing JWT path) or an `Authorization: Bearer <key>` **full-scope**
// API key, storing the resolved user id in the same locals as RequireAuth — so every
// handler behind it works unchanged. The cookie is tried first, leaving the
// browser path identical; a missing or invalid cookie falls through to the key.
// It responds 401 when neither credential resolves.
//
// Full-scope-only is the default on purpose: a route added later is confined to
// unrestricted credentials unless it deliberately opts into RequireAuthOrScopedKey, so a
// narrow agent credential can never reach a new endpoint by accident.
func RequireAuthOrKey(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return RequireAuthOrScopedKey(iss, versions, keys)
}

// RequireAuthOrScopedKey is RequireAuthOrKey that additionally admits keys carrying one of
// the given narrow scopes. A full-scope key is always admitted. A live key whose scope is
// not admitted here is 403 (insufficient scope) rather than 401 — the credential is real,
// it is simply not allowed on this surface, and conflating the two would send an agent into
// a re-authentication loop it can never win.
func RequireAuthOrScopedKey(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator, allowed ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, ok := resolveSession(c, iss, versions, token); ok {
				c.Locals(LocalsUserID, id)
				return c.Next()
			}
		}
		if b, ok := resolveBearer(c, iss, versions, keys); ok {
			if b.viaKey && !scopeAllowed(b.scope, allowed) {
				return fiber.NewError(fiber.StatusForbidden, "api key scope is insufficient for this endpoint")
			}
			c.Locals(LocalsUserID, b.userID)
			if b.viaKey {
				c.Locals(localsViaAPIKey, true)
				c.Locals(localsKeyScope, b.scope)
			}
			return c.Next()
		}
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
}

// scopeAllowed reports whether a key's scope may pass. ScopeFull passes everywhere;
// anything else must be listed explicitly by the route's middleware.
func scopeAllowed(scope string, allowed []string) bool {
	if scope == ScopeFull {
		return true
	}
	for _, a := range allowed {
		if scope == a {
			return true
		}
	}
	return false
}

// OptionalAuth returns middleware that attaches the caller's user id when a valid
// session cookie or API key is present, and otherwise passes through anonymously.
// It NEVER rejects: an absent, expired, or invalid credential simply leaves no user
// id in locals, so a public read still succeeds. Used on the job/company detail
// reads to overlay the caller's own vote without gating the page behind sign-in.
func OptionalAuth(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, ok := resolveSession(c, iss, versions, token); ok {
				c.Locals(LocalsUserID, id)
				return c.Next()
			}
		}
		// No scope gate here: this never rejects, and an under-scoped key on a public
		// read only enriches the response with its own owner's overlay.
		if b, ok := resolveBearer(c, iss, versions, keys); ok {
			c.Locals(LocalsUserID, b.userID)
			if b.viaKey {
				c.Locals(localsViaAPIKey, true)
				c.Locals(localsKeyScope, b.scope)
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
// when there is no bearer or neither interpretation resolves.
func resolveBearer(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) (bearerIdentity, bool) {
	return resolveCredential(c, iss, versions, keys, bearerToken(c))
}

// resolveCredential interprets one opaque credential string as either a session JWT or
// an API key. Split from resolveBearer because the websocket handshake carries the same
// credential in a subprotocol rather than a header, and both must agree on what a token
// means.
func resolveCredential(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator, tok string) (bearerIdentity, bool) {
	if tok == "" {
		return bearerIdentity{}, false
	}
	// A JWT bearer IS a session, so it takes the same revocation check as the cookie —
	// otherwise "sign out everywhere" would leave the extension's copy of the token
	// working, which is exactly the device a user reaches for that button to evict.
	if id, ok := resolveSession(c, iss, versions, tok); ok {
		return bearerIdentity{userID: id}, true
	}
	if identity, err := keys.AuthenticateAPIKey(c.Context(), HashAPIKey(tok)); err == nil {
		return bearerIdentity{userID: identity.UserID, viaKey: true, scope: identity.Scope}, true
	}
	return bearerIdentity{}, false
}

// bearerIdentity is what a Bearer credential resolved to. viaKey distinguishes an API
// key (scoped, flagged to handlers) from a session JWT (a full session, like the cookie).
type bearerIdentity struct {
	userID int64
	viaKey bool
	scope  string
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
