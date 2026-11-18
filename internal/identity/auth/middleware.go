package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// LocalsUserID is the c.Locals key for the authenticated user id. Exported because a
// websocket handler gets a Conn, not a Ctx, and reads the local by key.
const LocalsUserID = "auth.userID"

// localsSessionVersion holds the tv claim resolveSession validated. Set only there.
const localsSessionVersion = "auth.sessionVersion"

// localsViaAPIKey is set by RequireAuthOrKey for API-key requests, so handlers can give
// programmatic callers a narrower view than a browser session.
const localsViaAPIKey = "auth.viaAPIKey"

// localsKeyScope holds the authenticating key's scope; absent for unscoped cookie sessions.
const localsKeyScope = "auth.keyScope"

// localsViaCookie is set when the request authenticated with the website's session cookie
// rather than a Bearer credential (extension JWT or API key).
const localsViaCookie = "auth.viaCookie"

// Scopes an API key can carry. ScopeFull is the default for user-created keys; ScopeCV is
// only the CV surface, so a key handed to an external agent cannot reach the rest of the
// account. ScopeEmbed is validated by the volunteer-embed service directly against
// api_keys and grants nothing on this API.
const (
	ScopeFull  = "full"
	ScopeCV    = "cv"
	ScopeEmbed = "embed"
)

// ViaAPIKey reports whether the request authenticated via an API key. False for cookie
// auth and requests that skipped RequireAuthOrKey.
func ViaAPIKey(c *fiber.Ctx) bool {
	via, _ := c.Locals(localsViaAPIKey).(bool)
	return via
}

// ViaAPIKeyFromContext is ViaAPIKey for a contract handler holding the request context.
func ViaAPIKeyFromContext(ctx context.Context) bool {
	via, _ := ctx.Value(localsViaAPIKey).(bool)
	return via
}

// KeyScope returns the authenticating key's scope, or "" for a cookie session.
func KeyScope(c *fiber.Ctx) string {
	scope, _ := c.Locals(localsKeyScope).(string)
	return scope
}

// ViaCookie reports whether the request authenticated via the website's session cookie.
// False for a Bearer JWT, an API key, or a request that skipped the auth middleware.
func ViaCookie(c *fiber.Ctx) bool {
	via, _ := c.Locals(localsViaCookie).(bool)
	return via
}

// IsExtensionBearer reports whether the request authenticated as a Bearer session JWT,
// neither cookie nor API key: the browser extension's carrier, since it cannot send a
// cross-origin cookie. It is the single definition for confining a capability to the
// extension's own connection; do not re-derive it inline.
func IsExtensionBearer(c *fiber.Ctx) bool {
	return !ViaCookie(c) && !ViaAPIKey(c)
}

// IsExtensionBearerFromContext is IsExtensionBearer for a contract handler.
func IsExtensionBearerFromContext(ctx context.Context) bool {
	cookie, _ := ctx.Value(localsViaCookie).(bool)
	return !cookie && !ViaAPIKeyFromContext(ctx)
}

type sessionResult int

const (
	sessionOK sessionResult = iota
	sessionInvalid
	sessionRevoked
	sessionInfraError
)

// TokenVersionLoader resolves a user id to its current session generation (satisfied by
// *db.Queries.GetUserTokenVersion).
type TokenVersionLoader interface {
	GetUserTokenVersion(ctx context.Context, id int64) (int32, error)
}

// RequireAuth validates the auth cookie and stores the user id in locals. It responds 401
// on a missing, expired, invalid or revoked token and 503 on database errors.
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

// OptionalCookieAuth attaches the user id for a valid session cookie and otherwise passes
// through anonymously. It is OptionalAuth without the Bearer path, for surfaces a leaked
// API key must not reach.
//
// OAuth-style callbacks mount it instead of RequireAuth: they are top-level navigations,
// where a 401 would render JSON, so the handler redirects back with an error marker.
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

// resolveSession validates a cookie token and confirms it was not revoked.
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
	c.Locals(localsSessionVersion, version)
	return id, sessionOK, nil
}

// SessionVersion returns the generation the authenticated session was minted under (the
// tv claim the middleware validated). A handler recording session-scoped authority must
// bind this value, not a fresh read of users.token_version: a revocation landing in
// between would capture the new generation into a credential it meant to kill. Not ok
// when there is no session.
func SessionVersion(c *fiber.Ctx) (int32, error) {
	v, ok := c.Locals(localsSessionVersion).(int32)
	if !ok {
		return 0, ErrNoSession
	}
	return v, nil
}

// UserID returns the authenticated user id; false when the request skipped the middleware.
func UserID(c *fiber.Ctx) (int64, bool) {
	id, ok := c.Locals(LocalsUserID).(int64)
	return id, ok
}

// UserIDFromContext is UserID for a net/http handler behind Fiber's adaptor (the
// contract server); the adaptor passes the fasthttp RequestCtx on as the context.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(LocalsUserID).(int64)
	return id, ok
}

// APIKeyIdentity is what a live API key resolves to: its owner and scope.
type APIKeyIdentity struct {
	UserID     int64
	Scope      string
	KeyID      int64
	OAuthGrant bool
}

// APIKeyAuthenticator resolves a presented API-key hash to its owner and scope,
// returning an error when no live key matches.
type APIKeyAuthenticator interface {
	AuthenticateAPIKey(ctx context.Context, tokenHash string) (APIKeyIdentity, error)
}

// RequireAuthOrKey authenticates by the session cookie or a full-scope Bearer API key,
// storing the user id in the same locals as RequireAuth. The cookie is tried first; a
// missing or invalid one falls through to the key. 401 when neither resolves. A key-lookup
// failure other than pgx.ErrNoRows is returned, never masked as a 401.
//
// Full-scope-only is the default so a new route is closed to narrow agent credentials
// unless it opts into RequireAuthOrScopedKey.
func RequireAuthOrKey(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return RequireAuthOrScopedKey(iss, versions, keys)
}

// RequireAuthOrScopedKey is RequireAuthOrKey that also admits keys with one of the given
// scopes. A live key with a non-admitted scope gets 403, not 401, so an agent does not
// loop on re-authentication it can never win.
func RequireAuthOrScopedKey(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator, allowed ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "unknown"})
		if token := c.Cookies(CookieName); token != "" {
			id, res, _ := resolveSession(c, iss, versions, token)
			if res == sessionOK {
				c.Locals(LocalsUserID, id)
				c.Locals(localsViaCookie, true)
				observability.IdentifyRequest(c, observability.RequestIdentity{Kind: "session", UserID: id})
				return c.Next()
			}
			if res == sessionInfraError {
				observability.LogAuthFailure(c, fiber.StatusServiceUnavailable, "session_lookup_failed")
				return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
			}
		}
		b, ok, err := resolveBearer(c, iss, versions, keys)
		if err != nil {
			observability.LogAuthFailure(c, fiber.StatusServiceUnavailable, "credential_lookup_failed")
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		if ok {
			observability.IdentifyRequest(c, observability.RequestIdentity{Kind: b.credentialType, UserID: b.userID, KeyID: b.keyID, Scope: b.scope})
			if b.viaKey && !scopeAllowed(b.scope, allowed) {
				observability.LogAuthFailure(c, fiber.StatusForbidden, "insufficient_scope")
				return fiber.NewError(fiber.StatusForbidden, "api key scope is insufficient for this endpoint")
			}
			c.Locals(LocalsUserID, b.userID)
			if b.viaKey {
				c.Locals(localsViaAPIKey, true)
				c.Locals(localsKeyScope, b.scope)
			}
			return c.Next()
		}
		reason := "invalid_or_expired_credential"
		if c.Get(fiber.HeaderAuthorization) == "" && c.Cookies(CookieName) == "" {
			reason = "missing_credential"
		}
		observability.LogAuthFailure(c, fiber.StatusUnauthorized, reason)
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
}

// scopeAllowed reports whether a key's scope may pass; ScopeFull always, others if listed.
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

// OptionalAuth attaches the user id for a valid session cookie or API key and otherwise
// passes through anonymously, so public reads still succeed.
//
// Cookie DB errors degrade to guest mode to keep public reads up in an outage; a Bearer
// API key lookup failure returns 503 so programmatic clients get an actionable status.
func OptionalAuth(iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token := c.Cookies(CookieName); token != "" {
			if id, res, _ := resolveSession(c, iss, versions, token); res == sessionOK {
				c.Locals(LocalsUserID, id)
				return c.Next()
			}
		}
		// No scope gate: an under-scoped key on a public read only adds its owner's overlay.
		// A key lookup failure is returned as 503, not degraded.
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

// RoleLoader resolves a user id to its current role (satisfied by *db.Queries.GetUserRole).
type RoleLoader interface {
	GetUserRole(ctx context.Context, id int64) (string, error)
}

// RequireRole authorizes by the caller's role. It runs after an authentication middleware
// and reads the role fresh from the database so a change takes effect immediately. It
// fails closed: a missing user id or role-load error is 401, a mismatched role 403.
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

// resolveBearer authenticates a Bearer credential, a session JWT or an API key. A JWT
// bearer (the extension) is a full session; only an API key sets viaKey. ok=false when
// nothing resolves. A key-lookup failure other than pgx.ErrNoRows is returned, not masked.
func resolveBearer(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator) (bearerIdentity, bool, error) {
	return resolveCredential(c, iss, versions, keys, bearerToken(c))
}

// resolveCredential interprets one credential as a session JWT or an API key. Split from
// resolveBearer because the websocket handshake carries it in a subprotocol.
func resolveCredential(c *fiber.Ctx, iss *Issuer, versions TokenVersionLoader, keys APIKeyAuthenticator, tok string) (bearerIdentity, bool, error) {
	if tok == "" {
		return bearerIdentity{}, false, nil
	}
	id, res, err := resolveSession(c, iss, versions, tok)
	if res == sessionOK {
		return bearerIdentity{userID: id, credentialType: "session"}, true, nil
	}
	if res == sessionInfraError {
		return bearerIdentity{}, false, err
	}
	identity, err := keys.AuthenticateAPIKey(c.Context(), HashAPIKey(tok))
	switch {
	case err == nil:
		kind := "api_key"
		if identity.OAuthGrant {
			kind = "oauth_grant"
		}
		return bearerIdentity{userID: identity.UserID, viaKey: true, scope: identity.Scope, keyID: identity.KeyID, credentialType: kind}, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		// No live key matches.
		return bearerIdentity{}, false, nil
	default:
		// A real lookup failure: surface it.
		return bearerIdentity{}, false, err
	}
}

// bearerIdentity is what a Bearer credential resolved to; viaKey marks an API key as
// opposed to a session JWT.
type bearerIdentity struct {
	userID         int64
	viaKey         bool
	scope          string
	keyID          int64
	credentialType string
}

// bearerToken extracts the Bearer credential, or "" when absent.
func bearerToken(c *fiber.Ctx) string {
	const prefix = "Bearer "
	h := c.Get(fiber.HeaderAuthorization)
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		// Trim whitespace a client or proxy added so a valid key still matches.
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
