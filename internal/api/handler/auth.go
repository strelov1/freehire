package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/platform/db"
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
	queries       *db.Queries
	issuer        *auth.Issuer
	cookieSecure  bool
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
	// servedHosts are the exact hostnames this deployment answers on. Only these are
	// honoured as an OAuth redirect origin; anything else falls back to frontendOrigin.
	// It is deliberately NOT cookieDomains: a suffix match is right for cookie scope
	// and wrong for a redirect target (see requestOrigin).
	servedHosts     []string
	accounts        *accounts.Service
	throttler       ratelimit.Throttler
	authV2Enabled   bool
	mobileCallbacks map[string]string
	mobileAuth      *mobileauth.Store
	recentAuth      *recentauth.Store
	appleNative     *appleauth.Client
	appleGrantKeys  *appleauth.KeyRing
	// pushNotifier sends test/device push notifications (see me_push_tokens.go).
	// pushnotify.Notifier in production; a fake in tests.
	pushNotifier pushnotify.Notifier
	// accountDelete erases an account for good — rows, objects, and the Gmail grant.
	// accountEmails resolves the caller's own email for the typed confirmation. Both
	// are nil until withAccountDeletion wires them.
	accountDelete accountEraser
	accountEmails accountEmailLookup
}

// withAccountDeletion wires the erase-my-account path. It is set after construction
// because the eraser needs the Gmail revoker, which belongs to the inbox handlers.
func (h *authHandlers) withAccountDeletion(eraser accountEraser, emails accountEmailLookup) {
	h.accountDelete = eraser
	h.accountEmails = emails
}

func newAuthHandlers(queries *db.Queries, pool *pgxpool.Pool, throttler ratelimit.Throttler, issuer *auth.Issuer, cookieSecure bool, cookieDomains []string, providers oauthRegistry, frontendOrigin string, extensionRedirectAllowlist []string, servedHosts []string) *authHandlers {
	pushStore := pushnotify.NewQueriesStore(queries)
	return &authHandlers{
		queries:                    queries,
		issuer:                     issuer,
		cookieSecure:               cookieSecure,
		cookieDomains:              cookieDomains,
		oauth:                      providers,
		oauthCodes:                 oauth.NewCodeStore(60 * time.Second),
		frontendOrigin:             frontendOrigin,
		extensionRedirectAllowlist: extensionRedirectAllowlist,
		servedHosts:                servedHosts,
		accounts:                   accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
		throttler:                  throttler,
		pushNotifier:               pushnotify.NewExpoNotifier(pushStore, pushStore, pushStore),
	}
}

func (h *authHandlers) register(api fiber.Router, mw middleware) {
	noCache := func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		c.Set("Pragma", "no-cache")
		return c.Next()
	}

	// API-key management is cookie-only (RequireAuth): a leaked key must not be
	// able to create, list, or revoke keys. The create endpoint returns the
	// plaintext token exactly once.
	// Changing the password is cookie-only for the same reason: a leaked credential
	// must not be able to change the credential it would outlive.
	meGroup := api.Group("/me", noCache)
	recent := func(c *fiber.Ctx) error { return c.Next() }
	if h.authV2Enabled {
		recent = h.requireRecentAuth
	}
	meGroup.Patch("/timezone", mw.cookie, h.UpdateTimezone)
	meGroup.Patch("/language", mw.cookie, h.UpdateLanguage)
	meGroup.Get("/username", mw.cookie, h.GetUsername)
	meGroup.Put("/username", mw.cookie, h.ClaimUsername)
	meGroup.Post("/password", mw.cookie, recent, h.ChangePassword)
	meGroup.Post("/api-keys", mw.cookie, recent, h.CreateAPIKey)
	meGroup.Get("/api-keys", mw.cookie, h.ListAPIKeys)
	meGroup.Delete("/api-keys/:id", mw.cookie, recent, h.RevokeAPIKey)

	// Push-token registration is cookie-only for the same reason key management
	// is: a leaked API key must not be able to redirect another device's push
	// notifications to itself. The self-test send only ever targets the
	// caller's own registered token(s) — see TestPushToken — and is rate-limited
	// since it calls out to Expo's push API once per registered device.
	meGroup.Post("/push-tokens", mw.cookie, h.RegisterPushToken)
	meGroup.Get("/push-tokens", mw.cookie, h.ListPushTokens)
	meGroup.Delete("/push-tokens", mw.cookie, h.UnregisterPushToken)
	meGroup.Post("/push-tokens/test", mw.cookie, testPushTokenLimiter(h.throttler), h.TestPushToken)

	// The in-app notification center: a durable, channel-independent record of
	// every delivered subscription digest/reminder/nudge. Cookie-only, same
	// reasoning as push-token management.
	meGroup.Get("/notifications", mw.cookie, h.GetNotifications)
	meGroup.Get("/notifications/:id", mw.cookie, h.GetNotification)
	meGroup.Post("/notifications/:id/read", mw.cookie, h.MarkNotificationRead)
	meGroup.Post("/notifications/read-all", mw.cookie, h.MarkAllNotificationsRead)

	// Account deletion is permanent and cookie-only, for the same reason key
	// management is: a leaked API key must not be able to destroy the account that
	// issued it. The body confirms the caller's own email address. It requires
	// recent auth for the same reason password change does: a hijacked session
	// must not be able to take the one action that can't be undone.
	meGroup.Delete("", mw.cookie, recent, h.DeleteAccount)

	// Availability check for a candidate username: public, so a name picker can
	// probe before the caller is even signed in.
	api.Get("/username/check", h.CheckUsername)

	// Auth: register/login/logout are public (logout just clears the cookie).
	// me is guarded and accepts a session cookie OR an API key, so a non-browser
	// client (e.g. the CLI) can resolve its own identity with its key. It stays a
	// read of the caller's own user — not key management, which is cookie-only.
	// Throttle credential endpoints against online brute-force / credential stuffing.
	loginLimiter := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("login"), 10, time.Minute)
	registerLimiter := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("register"), 5, time.Minute)
	verifyReqLimiter := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("verify-req"), 5, time.Minute)
	verifyConfLimiter := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("verify-conf"), 10, 15*time.Minute)
	forgotLimiter := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("forgot"), 3, 5*time.Minute)
	resetLimiter := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("reset"), 10, 15*time.Minute)

	authGroup := api.Group("/auth", noCache)
	authGroup.Post("/register", registerLimiter, h.Register)
	authGroup.Post("/login", loginLimiter, h.Login)
	authGroup.Post("/logout", h.Logout)
	// Email verification. Cookie-only and identified by the session, never by a body
	// field, so neither endpoint can be pointed at someone else's address. Both ride the
	// credential limiter: confirm is a code-guessing surface, request is a mail-sending one.
	authGroup.Post("/verify/request", verifyReqLimiter, mw.cookie, h.RequestEmailVerification)
	authGroup.Post("/verify/confirm", verifyConfLimiter, mw.cookie, h.ConfirmEmailVerification)
	// Password recovery. Public — the mailed code is the credential. forgot always answers
	// 202 (never an enumeration oracle); reset revokes every session on success.
	authGroup.Post("/password/forgot", forgotLimiter, h.ForgotPassword)
	authGroup.Post("/password/reset", resetLimiter, h.ResetPassword)
	// Sign out everywhere. Cookie-only (like key management): revoking a human's sessions
	// is not something a programmatic credential should be able to do.
	authGroup.Post("/logout-all", mw.cookie, h.LogoutAll)
	// The agent needs to resolve who its key belongs to, so the identity read admits the
	// narrow scope too; it exposes nothing the key holder cannot already infer.
	authGroup.Get("/me", mw.cvKey, h.Me)

	// OAuth sign-in: provider listing plus the authorization-code start and
	// callback redirects. All public; the callback sets the session cookie.
	authGroup.Get("/oauth/providers", h.ListOAuthProviders)
	authGroup.Get("/oauth/:provider/start", h.OAuthStart)
	authGroup.Get("/oauth/:provider/callback", h.OAuthCallback)
	// Apple's callback arrives as a POST with a form-encoded body (mandatory
	// once the email scope is requested); every other provider uses the GET above.
	authGroup.Post("/oauth/:provider/callback", h.OAuthCallback)
	// Mobile-only: redeem the one-time code from the custom-scheme callback for a
	// session. Public; the code is the credential.
	authGroup.Post("/oauth/exchange", h.OAuthExchange)

	// Browser-extension sign-in ("Sign in with freehire"): the extension opens
	// this in the freehire origin via launchWebAuthFlow. Cookie-only — a leaked key
	// must not mint further keys — but on optionalCookie, like the OAuth callbacks,
	// because these are browser navigations too: Chrome's auth window has its own
	// cookie jar, so arriving with no session is the normal first run, and a 401
	// body there is what it reports as "Authorization page could not be loaded".
	// Each handler sends a sessionless visitor to sign in instead. GET shows the
	// consent screen; POST issues a session JWT and redirects the token in the
	// fragment. Both refuse any redirect outside the configured allowlist.
	authGroup.Get("/extension/connect", mw.optionalCookie, h.ExtensionConnect)
	authGroup.Post("/extension/connect", mw.optionalCookie, h.ExtensionConnectSubmit)
}

func (h *authHandlers) requireRecentAuth(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	version, err := h.queries.GetUserTokenVersion(c.Context(), userID)
	if err != nil {
		return err
	}
	sessionHash, fingerprintErr := h.currentSessionHash(c)
	if fingerprintErr != nil {
		return authError(428, "recent_auth_required", "recent authentication required")
	}
	if err := h.recentAuth.Validate(c.Context(), c.Cookies(recentauth.CookieName), userID, version, sessionHash); err != nil {
		if errors.Is(err, recentauth.ErrRequired) {
			return authError(428, "recent_auth_required", "recent authentication required")
		}
		return err
	}
	return c.Next()
}

// userResponse is the public shape of a user. It deliberately omits
// password_hash so the hash never reaches a response. role is included so the SPA can
// decide whether to surface moderator-only UI; it is an affordance only, as RequireRole
// re-checks the DB-stored role on every privileged request.
type userResponse struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	BetaTester bool   `json:"beta_tester"`
	// EmailVerified drives the SPA's "confirm your email" prompt. It is an affordance
	// only: the server never trusts the client's copy of it.
	EmailVerified bool `json:"email_verified"`
	// HasPassword lets the SPA offer a password change to password accounts and explain
	// itself to OAuth-only ones, instead of guessing from "is signed in".
	HasPassword bool       `json:"has_password"`
	CreatedAt   *time.Time `json:"created_at"`
	// Timezone is the account's IANA name (nil until set), read by the profile
	// page to pre-fill its timezone field and by the notification-settings UI's
	// missing-timezone hint.
	Timezone *string `json:"timezone"`
	// Language is the account's preferred interface language (one of
	// supportedLanguages, "en" until changed). No UI translation exists yet —
	// this only records the preference for later (freehire#1836).
	Language string `json:"language"`
	// OnboardingCompletedAt is when this account finished (or declined) the onboarding
	// wizard, null for one that never has. The root layout reads it as the whole gate:
	// null routes the user to /onboarding, a timestamp never does. It rides along here
	// rather than on its own endpoint because the layout already makes this read before
	// it can decide anything at all.
	OnboardingCompletedAt *time.Time `json:"onboarding_completed_at"`
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Timezone is the browser-detected IANA zone, captured at registration only
	// (Login ignores it — an existing account's timezone is edited on the
	// profile page, not overwritten silently on every sign-in).
	Timezone *string `json:"timezone"`
}

// toUserResponse maps an accounts.User to its public response shape.
func toUserResponse(u accounts.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, BetaTester: u.BetaTester,
		EmailVerified: u.EmailVerified, HasPassword: u.HasPassword, CreatedAt: u.CreatedAt,
		Timezone: u.Timezone, Language: u.Language, OnboardingCompletedAt: u.OnboardingCompletedAt}
}

// timezoneRequest is the PATCH /me/timezone body.
type timezoneRequest struct {
	Timezone string `json:"timezone"`
}

// UpdateTimezone sets the caller's IANA timezone name. Cookie-only.
func (h *authHandlers) UpdateTimezone(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in timezoneRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := h.accounts.UpdateTimezone(c.Context(), userID, in.Timezone)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// languageRequest is the PATCH /me/language body.
type languageRequest struct {
	Language string `json:"language"`
}

// UpdateLanguage sets the caller's preferred interface language. Cookie-only.
func (h *authHandlers) UpdateLanguage(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in languageRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := h.accounts.UpdateLanguage(c.Context(), userID, in.Language)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
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
	case errors.Is(err, accounts.ErrInvalidTimezone):
		return fiber.NewError(fiber.StatusBadRequest, "invalid timezone")
	case errors.Is(err, accounts.ErrInvalidLanguage):
		return fiber.NewError(fiber.StatusBadRequest, "invalid language")
	case errors.Is(err, accounts.ErrUsernameInvalid):
		return fiber.NewError(fiber.StatusBadRequest, "invalid username")
	case errors.Is(err, accounts.ErrUsernameTaken):
		return fiber.NewError(fiber.StatusConflict, "username already taken")
	case errors.Is(err, accounts.ErrUsernameReserved):
		return fiber.NewError(fiber.StatusConflict, "username is reserved")
	case errors.Is(err, accounts.ErrUsernameCooldown):
		return fiber.NewError(fiber.StatusTooManyRequests, "username changed too recently")
	default:
		if isInfraError(err) {
			return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
		}
		return err
	}
}

func isInfraError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection") || strings.Contains(msg, "closed") || strings.Contains(msg, "timeout") || strings.Contains(msg, "refusal") || strings.Contains(msg, "pool")
}

// Register creates an account, starts a session (auth cookie), and returns the
// user.
func (h *authHandlers) Register(c *fiber.Ctx) error {
	var in credentials
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	user, err := h.accounts.Register(c.Context(), in.Email, in.Password, in.Timezone)
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
	var revokeErr error
	if h.recentAuth != nil {
		revokeErr = h.recentAuth.Revoke(c.Context(), c.Cookies(recentauth.CookieName))
	}
	domain := auth.CookieDomainForHost(c.Hostname(), h.cookieDomains)
	auth.ClearTokenCookie(c, h.cookieSecure, domain)
	recentauth.ClearCookie(c, h.cookieSecure, domain)
	if revokeErr != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// LogoutAll revokes every session for the caller's account — including this one — by
// advancing the account's token generation, then clears the caller's cookie. Cookie-only:
// a leaked API key must not be able to sign the owner out of their browser, and an agent
// has no business ending human sessions.
func (h *authHandlers) LogoutAll(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := h.queries.BumpUserTokenVersion(c.Context(), userID); err != nil {
		return err
	}
	domain := auth.CookieDomainForHost(c.Hostname(), h.cookieDomains)
	auth.ClearTokenCookie(c, h.cookieSecure, domain)
	recentauth.ClearCookie(c, h.cookieSecure, domain)
	return c.SendStatus(fiber.StatusNoContent)
}

// setSession issues a token for userID and writes it as the auth cookie. The token is
// stamped with the account's current session generation, so it is born valid and any
// later revocation strands it.
func (h *authHandlers) setSession(c *fiber.Ctx, userID int64) error {
	version, err := h.queries.GetUserTokenVersion(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
	}
	return h.setSessionAt(c, userID, version)
}

// setSessionAt writes a session cookie for a known generation. Callers that just bumped
// the counter (password change, sign-out-everywhere) pass the value they got back, so the
// caller's replacement cookie cannot race the bump they themselves performed.
func (h *authHandlers) setSessionAt(c *fiber.Ctx, userID int64, version int32) error {
	_, err := h.issueSessionAt(c, userID, version)
	return err
}

func (h *authHandlers) issueSessionAt(c *fiber.Ctx, userID int64, version int32) (string, error) {
	token, err := h.issuer.Issue(userID, version)
	if err != nil {
		return "", fiber.NewError(fiber.StatusInternalServerError, "failed to start session")
	}
	auth.SetTokenCookie(c, token, h.issuer.TTL(), h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return token, nil
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

// apiKeys adapts the generated query row to auth.APIKeyIdentity. The auth package stays
// free of a database import, which is why it cannot name the sqlc row type directly.
type apiKeys struct{ q *db.Queries }

func (a apiKeys) AuthenticateAPIKey(ctx context.Context, tokenHash string) (auth.APIKeyIdentity, error) {
	row, err := a.q.AuthenticateAPIKey(ctx, tokenHash)
	if err != nil {
		return auth.APIKeyIdentity{}, err
	}
	return auth.APIKeyIdentity{UserID: row.UserID, Scope: row.Scope}, nil
}
