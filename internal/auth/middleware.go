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

// localsKeyScope holds the authenticating key's scope. Absent for cookie sessions, which
// are unscoped by construction — a browser session is the owner themselves.
const localsKeyScope = "auth.keyScope"

// localsViaCookie is set true whenever the request authenticated with the website's own
// session cookie, as opposed to a Bearer credential (a session JWT — the browser
// extension's carrier — or an API key). Handlers read it via ViaCookie to confine a
// capability to the extension's own connection, the same way localsViaAPIKey narrows one
// to a programmatic caller.
const localsViaCookie = "auth.viaCookie"

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

// ViaCookie reports whether the request authenticated via the website's session cookie,
// as opposed to a Bearer credential. False for a Bearer session JWT, false for an API
// key, and false for a request that did not pass through RequireAuth or
// RequireAuthOrScopedKey.
func ViaCookie(c *fiber.Ctx) bool {
	via, _ := c.Locals(localsViaCookie).(bool)
	return via
}

// IsExtensionBearer reports whether the request authenticated as a Bearer session
// JWT — neither the website's cookie nor an API key. That carrier is the browser
// extension's own: it holds the session JWT the connect flow minted and presents it
// as a Bearer header because a browser cannot send a cross-origin cookie.
//
// This is the single definition every caller that must confine a capability to the
// extension's own connection shares (the assistant's browse-preset page tool,
// agent-driven autofill — both reach a browser-tool channel keyed by user id, not by
// how the request that reached it authenticated). Re-deriving "not cookie, not API
// key" inline at each call site is exactly the kind of duplication that drifts the
// moment one of them is edited and the other is not.
func IsExtensionBearer(c *fiber.Ctx) bool {
	return !ViaCookie(c) && !ViaAPIKey(c)
}

type sessionResult int

const (
	sessionOK sessionResult = iota
	sessionInvalid
	sessionRevoked
	sessionInfraError
)

// TokenVersionLoader resolves an authenticated user id to its current session
// generation. Like RoleLoader it returns a primitive so this package needs no database
// import; it is satisfied directly by *db.Queries (GetUserTokenVersion).
type TokenVersionLoader interface {
	GetUserTokenVersion(ctx context.Context, id int64) (int32, error)
}

// RequireAuth returns middleware that validates the auth cookie and stores the
// resolved user id in the request locals. It responds 401 on a missing,
// expired, invalid, or revoked token, and 503 on database infrastructure errors.
func RequireAuth(iss *Issuer, versions TokenVersionLoader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies(CookieName)
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		id, res, _ := resolveSession(c, iss, versions, token)
		if res == sessionInfraError {
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		if res != sessionOK {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired session")
		}
		c.Locals(LocalsUserID, id)
		c.Locals(localsViaCookie, true)
		return c.Next()
	}
}

// OptionalCookieAuth returns middleware that attaches the caller's user id when a
// valid session cookie is present and otherwise passes through anonymously. It is
// OptionalAuth without the Bearer path: for a surface a leaked API key must not
// reach (the mail inbox), admitting a key here would widen that key's reach for no
// gain — the only caller is a browser returning from a provider redirect, which
// carries a cookie or nothing.
//
// The OAuth-style callbacks mount this rather than RequireAuth: they are top-level
// navigations, so a 401 renders JSON into the browser and strands the user, while an
// anonymous pass-through lets the handler redirect back with an error marker.
func OptionalCookieAuth(iss *Issuer, versions TokenVersionLoader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, res, _ := resolveSession(c, iss, versions, token); res == sessionOK {
				c.Locals(LocalsUserID, id)
			}
		}
		return c.Next()
	}
}

// resolveSession validates a cookie token and confirms it was not revoked, returning the
// user id it authenticates and a classification result.
func resolveSession(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, token string) (int64, sessionResult, error) {
	id, version, err := iss.Parse(token)
	if err != nil {
		return 0, sessionInvalid, nil
	}
	current, err := versions.GetUserTokenVersion(c.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, sessionRevoked, nil
		}
		return 0, sessionInfraError, err
	}
	if current != version {
		return 0, sessionRevoked, nil
	}
	return id, sessionOK, nil
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
// It responds 401 when neither credential resolves. A key-lookup failure that is
// not "no such key" (pgx.ErrNoRows) is a real error and is returned — it must not
// be masked as a 401.
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
			id, res, _ := resolveSession(c, iss, versions, token)
			if res == sessionOK {
				c.Locals(LocalsUserID, id)
				c.Locals(localsViaCookie, true)
				return c.Next()
			}
			if res == sessionInfraError {
				return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
			}
		}
		b, ok, err := resolveBearer(c, iss, versions, keys)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		if ok {
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
// An absent, expired, or unknown credential simply leaves no user id in locals, so
// a public read still succeeds. Used on the job/company detail reads to overlay the
// caller's own vote without gating the page behind sign-in.
//
// Asymmetry Note: Cookie DB errors silently degrade to guest mode so public feed access
// is preserved during database outages. Conversely, explicit Bearer API key lookup failures
// (e.g. DB outage) return HTTP 503 so programmatic API clients receive actionable status.
func OptionalAuth(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, res, _ := resolveSession(c, iss, versions, token); res == sessionOK {
				c.Locals(LocalsUserID, id)
				return c.Next()
			}
		}
		// No scope gate here: an under-scoped key on a public read only enriches the
		// response with its own owner's overlay. A presented Bearer key lookup failure
		// (DB down, etc.) is real and returned as 503 — it must not be silently degraded.
		b, ok, err := resolveBearer(c, iss, versions, keys)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		if ok {
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
			if errors.Is(err, pgx.ErrNoRows) {
				return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
			}
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		if got != role {
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
func resolveBearer(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) (bearerIdentity, bool, error) {
	return resolveCredential(c, iss, versions, keys, bearerToken(c))
}

// resolveCredential interprets one opaque credential string as either a session JWT or
// an API key. Split from resolveBearer because the websocket handshake carries the same
// credential in a subprotocol rather than a header, and both must agree on what a token
// means.
func resolveCredential(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator, tok string) (bearerIdentity, bool, error) {
	if tok == "" {
		return bearerIdentity{}, false, nil
	}
	id, res, err := resolveSession(c, iss, versions, tok)
	if res == sessionOK {
		return bearerIdentity{userID: id}, true, nil
	}
	if res == sessionInfraError {
		return bearerIdentity{}, false, err
	}
	identity, err := keys.AuthenticateAPIKey(c.Context(), HashAPIKey(tok))
	switch {
	case err == nil:
		return bearerIdentity{userID: identity.UserID, viaKey: true, scope: identity.Scope}, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		// No live key matches — no credential resolved.
		return bearerIdentity{}, false, nil
	default:
		// A lookup failure (DB down, etc.) is real: surface it to the caller.
		return bearerIdentity{}, false, err
	}
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
