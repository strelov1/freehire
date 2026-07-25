package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/auth"
)

// oauthRegistry resolves OAuth providers by name, building each with a callback
// rooted at the request origin so the flow can complete on any served domain.
// *oauth.Registry implements it; tests supply a fake.
type oauthRegistry interface {
	Names() []string
	Provider(name, origin string) (oauth.Provider, bool)
}

// authHandlers serves the auth surface: register/login/logout/me, OAuth sign-in
// (provider listing, start/callback redirects, the mobile code exchange), the
// browser-extension connect flow, and API-key management. accounts resolves
// external OAuth identities into local user accounts (identity-first lookup,
// verified-email gate, link-or-create, race retry). cookieDomains are the
// registrable domains we serve (bare, e.g. "freehire.me"): per request the session
// cookie is scoped to whichever one the request host falls under (empty list =
// host-only, for dev); the same set gates requestOrigin (see
// auth.CookieDomainForHost).
type authHandlers struct {
	queries      *db.Queries
	issuer       *auth.Issuer
	cookieSecure bool
	cookieDomains []string
	// oauth resolves enabled OAuth providers by name, building each with a redirect
	// URL rooted at the request origin. Never nil (an empty registry 404s / lists
	// empty). *oauth.Registry in production; a fake in tests.
	oauth oauthRegistry
	// oauthCodes hands out the single-use codes that carry a mobile OAuth
	// sign-in from the browser callback to the app's /exchange call.
	oauthCodes *oauth.CodeStore
	// frontendOrigin is where OAuth callbacks send the browser back to.
	frontendOrigin string
	// extensionRedirectAllowlist bounds the browser-extension connect flow to the
	// chromiumapp.org redirect ids listed here. Empty refuses every redirect.
	extensionRedirectAllowlist []string
	accounts *accounts.Service
}

func newAuthHandlers(queries *db.Queries, pool *pgxpool.Pool, issuer *auth.Issuer, cookieSecure bool, cookieDomains []string, providers oauthRegistry, frontendOrigin string, extensionRedirectAllowlist []string) *authHandlers {
	return &authHandlers{
		queries:                    queries,
		issuer:                     issuer,
		cookieSecure:               cookieSecure,
		cookieDomains:              cookieDomains,
		oauth:                      providers,
		oauthCodes:                 oauth.NewCodeStore(60 * time.Second),
		frontendOrigin:             frontendOrigin,
		extensionRedirectAllowlist: extensionRedirectAllowlist,
		accounts:                   accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
	}
}

func (h *authHandlers) register(api fiber.Router, mw middleware) {
	// API-key management is cookie-only (RequireAuth): a leaked key must not be
	// able to create, list, or revoke keys. The create endpoint returns the
	// plaintext token exactly once.
	api.Post("/me/api-keys", mw.cookie, h.CreateAPIKey)
	api.Get("/me/api-keys", mw.cookie, h.ListAPIKeys)
	api.Delete("/me/api-keys/:id", mw.cookie, h.RevokeAPIKey)

	// Auth: register/login/logout are public (logout just clears the cookie).
	// me is guarded and accepts a session cookie OR an API key, so a non-browser
	// client (e.g. the CLI) can resolve its own identity with its key. It stays a
	// read of the caller's own user — not key management, which is cookie-only.
	// Throttle the credential endpoints against online brute-force / credential
	// stuffing. Keyed on c.IP() (the real client, via the trusted-proxy config); the
	// per-instance in-memory window is enough friction for a single-node deployment.
	authLimiter := limiter.New(limiter.Config{Max: 10, Expiration: time.Minute})
	authGroup := api.Group("/auth")
	authGroup.Post("/register", authLimiter, h.Register)
	authGroup.Post("/login", authLimiter, h.Login)
	authGroup.Post("/logout", h.Logout)
	authGroup.Get("/me", mw.key, h.Me)

	// OAuth sign-in: provider listing plus the authorization-code start and
	// callback redirects. All public; the callback sets the session cookie.
	authGroup.Get("/oauth/providers", h.ListOAuthProviders)
	authGroup.Get("/oauth/:provider/start", h.OAuthStart)
	authGroup.Get("/oauth/:provider/callback", h.OAuthCallback)
	// Mobile-only: redeem the one-time code from the custom-scheme callback for a
	// session. Public; the code is the credential.
	authGroup.Post("/oauth/exchange", h.OAuthExchange)

	// Browser-extension sign-in ("Sign in with freehire"): the extension opens
	// this in the freehire origin via launchWebAuthFlow. Cookie-only (RequireAuth)
	// like key management — a leaked key must not mint further keys. GET shows the
	// consent screen; POST mints a named key and redirects the token in the
	// fragment. Both refuse any redirect outside the configured allowlist.
	authGroup.Get("/extension/connect", mw.cookie, h.ExtensionConnect)
	authGroup.Post("/extension/connect", mw.cookie, h.ExtensionConnectSubmit)
}

// userResponse is the public shape of a user. It deliberately omits
// password_hash so the hash never reaches a response. role is included so the SPA can
// decide whether to surface moderator-only UI; it is an affordance only, as RequireRole
// re-checks the DB-stored role on every privileged request.
type userResponse struct {
	ID         int64      `json:"id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	BetaTester bool       `json:"beta_tester"`
	CreatedAt  *time.Time `json:"created_at"`
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// toUserResponse maps an accounts.User to its public response shape.
func toUserResponse(u accounts.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, BetaTester: u.BetaTester, CreatedAt: u.CreatedAt}
}

// accountsError maps the accounts service sentinels to HTTP errors, preserving
// the statuses and generic messages the handlers used before delegation.
func accountsError(err error) error {
	switch {
	case errors.Is(err, accounts.ErrInvalidEmail):
		return fiber.NewError(fiber.StatusBadRequest, "invalid email")
	case errors.Is(err, accounts.ErrPasswordTooShort):
		return fiber.NewError(fiber.StatusBadRequest, "password must be at least 8 characters")
	case errors.Is(err, accounts.ErrPasswordTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "password must be at most 72 characters")
	case errors.Is(err, accounts.ErrEmailTaken):
		return fiber.NewError(fiber.StatusConflict, "email already registered")
	case errors.Is(err, accounts.ErrInvalidCredentials):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, accounts.ErrUserNotFound):
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	default:
		return err
	}
}

// Register creates an account, starts a session (auth cookie), and returns the
// user.
func (h *authHandlers) Register(c *fiber.Ctx) error {
	var in credentials
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := h.accounts.Register(c.Context(), in.Email, in.Password)
	if err != nil {
		return accountsError(err)
	}
	if err := h.setSession(c, user.ID); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toUserResponse(user)})
}

// Login verifies credentials, starts a session (auth cookie), and returns the
// user. Unknown email, wrong password, and passwordless accounts all yield the
// same generic 401 so the response never reveals which factor failed.
func (h *authHandlers) Login(c *fiber.Ctx) error {
	var in credentials
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := h.accounts.Login(c.Context(), in.Email, in.Password)
	if err != nil {
		return accountsError(err)
	}
	if err := h.setSession(c, user.ID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// Logout clears the auth cookie. It is public and idempotent: clearing an
// absent or already-expired cookie is a no-op.
func (h *authHandlers) Logout(c *fiber.Ctx) error {
	auth.ClearTokenCookie(c, h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return c.SendStatus(fiber.StatusNoContent)
}

// setSession issues a token for userID and writes it as the auth cookie.
func (h *authHandlers) setSession(c *fiber.Ctx, userID int64) error {
	token, err := h.issuer.Issue(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to start session")
	}
	auth.SetTokenCookie(c, token, h.issuer.TTL(), h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return nil
}

// Me returns the authenticated user. It runs behind auth.RequireAuthOrKey (a
// session cookie or an API key), which has already resolved and stored the user id.
func (h *authHandlers) Me(c *fiber.Ctx) error {
	id, err := requireUserID(c)
	if err != nil {
		return err
	}
	user, err := h.accounts.UserByID(c.Context(), id)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// authHasher adapts the auth package's bcrypt helpers to the accounts.PasswordHasher
// interface, keeping the accounts package free of the auth/fiber dependency graph.
type authHasher struct{}

func (authHasher) Hash(plain string) (string, error) { return auth.HashPassword(plain) }
func (authHasher) Check(hash, plain string) error    { return auth.CheckPassword(hash, plain) }
