package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/ga4mp"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/identity/promo"
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
	// chromiumapp.org / extensions.allizom.org redirect ids listed here. Empty refuses every redirect.
	extensionRedirectAllowlist []string
	// servedHosts are the exact hostnames this deployment answers on. Only these are
	// honoured as an OAuth redirect origin; anything else falls back to frontendOrigin.
	// It is deliberately NOT cookieDomains: a suffix match is right for cookie scope
	// and wrong for a redirect target (see requestOrigin).
	servedHosts []string
	accounts    *accounts.Service
	// invites attributes a new account to whoever's link brought it. Optional and nil-safe:
	// attribution must never be able to fail a sign-up. See withInvites.
	invites *promo.Service

	authV2Enabled   bool
	mobileCallbacks map[string]string
	mobileAuth      *mobileauth.Store
	recentAuth      *recentauth.Store
	appleNative     *appleauth.Client
	appleGrantKeys  *appleauth.KeyRing
	// pushNotifier sends test/device push notifications (see me_push_tokens.go).
	// pushnotify.Notifier in production; a fake in tests.
	pushNotifier pushnotify.Notifier
	// ga4 sends the GA4 sign_up from the server (me_attribution.go). Nil — and so
	// inert — when GA4_API_SECRET is not configured.
	ga4 ga4mp.Reporter
	// browserOrigins are the origins a same-origin browser request may name in
	// Origin/Referer, consumed by requireSameOrigin (auth.BrowserOrigins over the
	// same frontendOrigin/cookieDomains pair the CORS allowlist is built from).
	browserOrigins []string
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

func newAuthHandlers(queries *db.Queries, pool *pgxpool.Pool, issuer *auth.Issuer, cookieSecure bool, cookieDomains []string, providers oauthRegistry, frontendOrigin string, extensionRedirectAllowlist []string, servedHosts []string) *authHandlers {
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
		servedHosts:                servedHostsOrDefault(servedHosts, frontendOrigin),
		accounts:                   accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
		pushNotifier:               pushnotify.NewExpoNotifier(pushStore, pushStore, pushStore),
		ga4:                        ga4Reporter(),
		browserOrigins:             auth.BrowserOrigins(frontendOrigin, cookieDomains, servedHosts...),
	}
}

// ga4Reporter is the env-configured GA4 sender, or nil. It returns the interface
// type explicitly: a nil *ga4mp.Client stored in a Reporter would not compare equal
// to nil, and the handler's "is it configured" check relies on that comparison.
func ga4Reporter() ga4mp.Reporter {
	if c := ga4mp.FromEnv(os.Getenv); c != nil {
		return c
	}
	return nil
}

func (h *authHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
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
	meGroup.Patch("/timezone", mw.cookie, contract)
	meGroup.Patch("/language", mw.cookie, contract)
	meGroup.Get("/username", mw.cookie, contract)
	meGroup.Put("/username", mw.cookie, contract)
	meGroup.Post("/password", mw.cookie, recent, h.ChangePassword)
	// Changing the account email: request mails a code to the NEW address, confirm moves
	// the account there. Cookie-only and recent-auth-gated on the request for the same
	// reason as the password — the address is a sign-in credential. Confirm needs no
	// fresh proof of its own: the code it spends could only be issued behind that gate.
	meGroup.Post("/email",
		mw.cookie, recent, contract)
	meGroup.Post("/email/confirm",
		mw.cookie, h.ConfirmEmailChange)
	meGroup.Post("/api-keys", mw.cookie, recent, contract)
	meGroup.Get("/api-keys", mw.cookie, contract)
	meGroup.Get("/api-plan", mw.cookie, contract)
	meGroup.Get("/api-usage", mw.cookie, contract)
	meGroup.Delete("/api-keys/:id", mw.cookie, recent, contract)
	// MCP OAuth "Connected devices": list is a plain read (same gating as
	// ListAPIKeys); revoke is recent-auth-gated, same reasoning as RevokeAPIKey.
	meGroup.Get("/oauth-grants", mw.cookie, contract)
	meGroup.Delete("/oauth-grants/:id", mw.cookie, recent, contract)

	// How a just-created account arrived (campaign tags, referring host, landing path),
	// reported once by the browser. Cookie-only; says whether GA4 was sent.
	meGroup.Post("/attribution", mw.cookie, contract)

	// Push-token registration is cookie-only for the same reason key management
	// is: a leaked API key must not be able to redirect another device's push
	// notifications to itself. The self-test send only ever targets the
	// caller's own registered token(s) — see TestPushToken.
	meGroup.Post("/push-tokens", mw.cookie, contract)
	meGroup.Get("/push-tokens", mw.cookie, contract)
	meGroup.Delete("/push-tokens", mw.cookie, contract)
	meGroup.Post("/push-tokens/test", mw.cookie, contract)

	// The in-app notification center: a durable, channel-independent record of
	// every delivered subscription digest/reminder/nudge. Cookie-only, same
	// reasoning as push-token management.
	meGroup.Get("/notifications", mw.cookie, contract)
	meGroup.Get("/notifications/:id", mw.cookie, contract)
	meGroup.Post("/notifications/:id/read", mw.cookie, contract)
	meGroup.Post("/notifications/read-all", mw.cookie, contract)

	// Account deletion is permanent and cookie-only, for the same reason key
	// management is: a leaked API key must not be able to destroy the account that
	// issued it. The body confirms the caller's own email address. It requires
	// recent auth for the same reason password change does: a hijacked session
	// must not be able to take the one action that can't be undone.
	meGroup.Delete("", mw.cookie, recent, h.DeleteAccount)

	// Availability check for a candidate username: public, so a name picker can
	// probe before the caller is even signed in.
	api.Get("/username/check", contract)

	// Auth: register/login/logout are public (logout just clears the cookie).
	// me is guarded and accepts a session cookie OR an API key, so a non-browser
	// client (e.g. the CLI) can resolve its own identity with its key. It stays a
	// read of the caller's own user — not key management, which is cookie-only.

	authGroup := api.Group("/auth", noCache)
	// sameOrigin binds session-issuing routes to the browser origins this deployment
	// serves (auth.BrowserOrigins, the same list the CORS allowlist is built from):
	// a present Origin or Referer must name one of them, and a Sec-Fetch-Site:
	// cross-site statement refuses even when both were stripped. Only the genuinely
	// headerless shape — curl, the CLI, a mobile SDK, any programmatic caller of the
	// documented JSON API — passes with no browser metadata at all. Mounted on the
	// session-COOKIE writers and nothing else: a cross-site form post to the
	// account-data routes arrives WITHOUT the session cookie (Lax transmits cookies
	// only on top-level navigations), so those routes need no origin gate; and
	// API-key callers of /auth/me send no Origin at all.
	sameOrigin := h.requireSameOrigin()
	authGroup.Post("/register", sameOrigin, h.Register)
	authGroup.Post("/login", sameOrigin, h.Login)
	authGroup.Post("/logout", sameOrigin, h.Logout)
	// Email verification. Cookie-only and identified by the session, never by a body
	// field, so neither endpoint can be pointed at someone else's address. Confirm is a code-guessing surface; request sends mail.
	authGroup.Post("/verify/request", mw.cookie, contract)
	authGroup.Post("/verify/confirm", mw.cookie, contract)
	// Password recovery. Public — the mailed code is the credential. forgot always answers
	// 202 (never an enumeration oracle); reset revokes every session on success.
	authGroup.Post("/password/forgot", contract)
	authGroup.Post("/password/reset", contract)
	// Sign out everywhere. Cookie-only (like key management): revoking a human's sessions
	// is not something a programmatic credential should be able to do, and sameOrigin
	// keeps a cross-site form from revoking the browser's sessions either.
	authGroup.Post("/logout-all", sameOrigin, mw.cookie, h.LogoutAll)
	// The agent needs to resolve who its key belongs to, so the identity read admits the
	// narrow scope too; it exposes nothing the key holder cannot already infer.
	authGroup.Get("/me", mw.cvKey, contract)

	// OAuth sign-in: provider listing plus the authorization-code start and
	// callback redirects. All public; the callback sets the session cookie.
	authGroup.Get("/oauth/providers", contract)
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

	// MCP OAuth 2.1 (freehire#3114). Mounted on api directly (NOT authGroup —
	// /api/v1/oauth/*, not /api/v1/auth/oauth/*): every external-facing string
	// that names these paths (the metadata document, the sign-in returnTo, the
	// consent form's own action) is written against that shorter path, and the
	// pre-existing /auth/oauth/* routes above are a different, unrelated
	// surface (social sign-in) that happens to share the "oauth" word.
	//
	// Dynamic client registration (RFC 7591): public, no session involved yet.
	api.Post("/oauth/register", noCache, h.RegisterOAuthClient)
	// Metadata discovery (RFC 8414 / RFC 9728): public and cacheable (no
	// noCache), proxied from /.well-known/* by the SvelteKit app — see
	// oauth_metadata.go for why a direct Go route at the well-known path
	// itself would never be reached.
	api.Get("/oauth/metadata/authorization-server", h.OAuthAuthorizationServerMetadata)
	api.Get("/oauth/metadata/protected-resource", h.OAuthProtectedResourceMetadata)
	api.Get("/oauth/metadata/protected-resource/account", h.OAuthAccountProtectedResourceMetadata)
	// The path-appended form some clients try after a 404 on the path-inserted one
	// (resource URL + "/.well-known/oauth-protected-resource"). Under /api, so it reaches
	// the API directly, not through the SvelteKit proxy.
	api.Get("/mcp/.well-known/oauth-protected-resource", h.OAuthProtectedResourceMetadata)
	api.Get("/mcp/account/.well-known/oauth-protected-resource", h.OAuthAccountProtectedResourceMetadata)
	// Authorize step: GET shows consent (sessionless visitors are sent to sign
	// in first), POST acts on the decision. optionalCookie on GET for the same
	// reason the extension flow uses it on its own GET — see oauth_authorize.go.
	api.Get("/oauth/authorize", noCache, mw.optionalCookie, h.OAuthAuthorize)
	api.Post("/oauth/authorize", noCache, mw.optionalCookie, h.OAuthAuthorizeSubmit)
	// Token exchange (PKCE code for a bearer access token): public — the code
	// and verifier are the credential here, not a session.
	api.Post("/oauth/token", noCache, h.OAuthToken)
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
	// Tier is the caller's resolved plan (plan.TierFree/TierPro/TierUltra). It rides
	// along on the same call that already powers currentUser() on every page — the
	// header's tier badge reads it from here rather than a second request to
	// GET /api/v1/me/plan just to learn what api.myPlan() would answer anyway.
	Tier plan.Tier `json:"tier"`
	// AgentConnected is whether the account holds a live API key or MCP grant — see
	// UserHasAgentCredential. The site's "connect your agent" strip stops asking once it
	// is true; riding along here lets the server leave the strip out of the first paint.
	AgentConnected bool `json:"agent_connected"`
	// APITier is the API product the account holds (billing.APITierOf), separate from
	// Tier: the account area shows its API workspace only to a holder.
	APITier billing.APITier `json:"api_tier"`
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Timezone is the browser-detected IANA zone, captured at registration only
	// (Login ignores it — an existing account's timezone is edited on the
	// profile page, not overwritten silently on every sign-in).
	Timezone *string `json:"timezone"`
}

// toUserResponse maps an accounts.User to its public response shape. Pure and DB-free on
// purpose — it leaves Tier at its zero value, which is why every response handler calls
// toUserResponseWithTier instead; this stays the thing submissions_test.go's
// TestToUserResponse_* cases exercise without a database.
func toUserResponse(u accounts.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, BetaTester: u.BetaTester,
		EmailVerified: u.EmailVerified, HasPassword: u.HasPassword, CreatedAt: u.CreatedAt,
		Timezone: u.Timezone, Language: u.Language, OnboardingCompletedAt: u.OnboardingCompletedAt}
}

// toUserResponseWithTier is what every response handler calls: toUserResponse plus the
// caller's resolved tier and agent connection, riding along on the same request that
// already powers currentUser() on every page — see resolveTiers.
func (h *authHandlers) toUserResponseWithTier(ctx context.Context, u accounts.User) userResponse {
	resp := toUserResponse(u)
	resp.Tier, resp.APITier = h.resolveTiers(ctx, u.ID)
	resp.AgentConnected = h.resolveAgentConnected(ctx, u.ID)
	return resp
}

// resolveAgentConnected degrades to false on a read failure or unset queries, for the
// reason resolveTiers gives: the worst it costs is the strip asking once more.
func (h *authHandlers) resolveAgentConnected(ctx context.Context, userID int64) bool {
	if h.queries == nil {
		return false
	}
	connected, err := h.queries.UserHasAgentCredential(ctx, userID)
	if err != nil {
		log.Printf("auth: resolving agent connection for user %d: %v", userID, err)
		return false
	}
	return connected
}

// resolveTiers reads one indexed row holding every entitlement and resolves the plan via
// plan.TierOf and the API product via billing.APITierOf — never a second resolution path.
// A read failure, or queries being unset (several handler unit tests build an
// authHandlers with only the fields their own code path touches), degrades to free and
// no API product and is logged rather than failing the response: nothing about signing
// in, registering, or reading one's own account may depend on a badge rendering correctly.
func (h *authHandlers) resolveTiers(ctx context.Context, userID int64) (plan.Tier, billing.APITier) {
	if h.queries == nil {
		return plan.TierFree, billing.APITierNone
	}
	row, err := h.queries.GetUserAPIEntitlement(ctx, userID)
	if err != nil {
		log.Printf("auth: resolving tier for user %d: %v", userID, err)
		return plan.TierFree, billing.APITierNone
	}
	now := time.Now().UTC()
	return plan.TierOf(row.ProUntil.Time, row.UltraUntil.Time, now), apiPlanFrom(row, now).Tier
}

// UpdateTimezone sets the caller's IANA timezone name. Cookie-only.
func (h *authHandlers) UpdateTimezone(ctx context.Context, req *apigen.AccountTimezoneUpdate) (*apigen.UpdateTimezoneOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := h.accounts.UpdateTimezone(ctx, userID, req.Timezone.Value, req.OnlyIfUnset.Value)
	if err != nil {
		return nil, accountsError(err)
	}
	return &apigen.UpdateTimezoneOK{Data: authUserOut(h.toUserResponseWithTier(ctx, user))}, nil
}

// UpdateLanguage sets the caller's preferred interface language. Cookie-only.
func (h *authHandlers) UpdateLanguage(ctx context.Context, req *apigen.AccountLanguageUpdate) (*apigen.UpdateLanguageOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := h.accounts.UpdateLanguage(ctx, userID, req.Language.Value)
	if err != nil {
		return nil, accountsError(err)
	}
	return &apigen.UpdateLanguageOK{Data: authUserOut(h.toUserResponseWithTier(ctx, user))}, nil
}

// authUserOut is the contract's shape of the account: the same fields userResponse writes
// for the routes that still set a cookie.
func authUserOut(r userResponse) apigen.AuthUser {
	return apigen.AuthUser{ID: r.ID, Email: r.Email, Role: r.Role, BetaTester: r.BetaTester,
		EmailVerified: r.EmailVerified, HasPassword: r.HasPassword, CreatedAt: nilDateTime(r.CreatedAt),
		Timezone: nilString(r.Timezone), Language: r.Language,
		OnboardingCompletedAt: nilDateTime(r.OnboardingCompletedAt),
		Tier:                  apigen.AuthUserTier(r.Tier), AgentConnected: r.AgentConnected,
		APITier: apigen.AuthUserAPITier(r.APITier)}
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
	case errors.Is(err, accounts.ErrAccountSuspended):
		return errAccountSuspended
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
		// Anything else is ours: classify answers a client cancel 499 and reports the rest.
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
	user, err := h.accounts.Register(c.Context(), in.Email, in.Password, in.Timezone)
	if err != nil {
		return accountsError(err)
	}
	if err := h.setSession(c, user.ID); err != nil {
		return err
	}
	// After the session, never before: the account is made and the person is signed in
	// whatever happens here.
	h.attributeInvite(c, user.ID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": h.toUserResponseWithTier(c.Context(), user)})
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
	return c.JSON(fiber.Map{"data": h.toUserResponseWithTier(c.Context(), user)})
}

// Logout clears the auth cookie. It is public and idempotent: clearing an
// absent or already-expired cookie is a no-op.
func (h *authHandlers) Logout(c *fiber.Ctx) error {
	if err := h.endSession(c); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// endSession revokes the recent-auth grant and clears both cookies.
func (h *authHandlers) endSession(c *fiber.Ctx) error {
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
	return nil
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
	version, err := h.sessionVersion(c.Context(), userID)
	if err != nil {
		return err
	}
	return h.setSessionAt(c, userID, version)
}

// errAccountSuspended is what every sign-in path answers for a banned account.
var errAccountSuspended = authError(fiber.StatusForbidden, "account_suspended", "This account is suspended")

// sessionVersion is the account's current session generation, the number a new session is
// stamped with. GetUserTokenVersion returns no row for a banned account, so this is the
// choke point that stops every path that ends in a session (password, OAuth, Apple, the
// mobile exchange, the extension connect) from signing one in; paths that hand out a
// code first call it to refuse early.
func (h *authHandlers) sessionVersion(ctx context.Context, userID int64) (int32, error) {
	version, err := h.queries.GetUserTokenVersion(ctx, userID)
	switch {
	case err == nil:
		return version, nil
	case errors.Is(err, pgx.ErrNoRows):
		return 0, errAccountSuspended
	default:
		return 0, fiber.NewError(fiber.StatusServiceUnavailable, "service temporarily unavailable")
	}
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
		// Returned rather than phrased as fiber.NewError(500, …): a signing failure is
		// a fault the error inbox must see with its cause, and a *fiber.Error carries
		// none. The caller sees the generic 500 body instead — see classify.
		return "", fmt.Errorf("issue session token for user %d: %w", userID, err)
	}
	auth.SetTokenCookie(c, token, h.issuer.TTL(), h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return token, nil
}

// GetAuthMe returns the authenticated user. It runs behind auth.RequireAuthOrKey (a
// session cookie or an API key), which has already resolved and stored the user id.
func (h *authHandlers) GetAuthMe(ctx context.Context) (*apigen.GetAuthMeOK, error) {
	id, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := h.accounts.UserByID(ctx, id)
	if err != nil {
		return nil, accountsError(err)
	}
	return &apigen.GetAuthMeOK{Data: authUserOut(h.toUserResponseWithTier(ctx, user))}, nil
}

// authHasher adapts the auth package's bcrypt helpers to the accounts.PasswordHasher
// interface, keeping the accounts package free of the auth/fiber dependency graph.
type authHasher struct{}

func (authHasher) Hash(plain string) (string, error) { return auth.HashPassword(plain) }
func (authHasher) Check(hash, plain string) error    { return auth.CheckPassword(hash, plain) }

// apiKeys adapts the generated query row to auth.APIKeyIdentity. The auth package stays
// free of a database import, which is why it cannot name the sqlc row type directly.
type apiKeys struct{ q *db.Queries }

// AuthenticateAPIKey resolves a bearer token hash against api_keys first, and —
// only when no live key matches — against oauth_grants (freehire#3114). The two
// tables are deliberately separate (a key and a grant are minted, listed, and
// revoked through different surfaces), but every REST route this authenticator
// guards must accept either: an MCP client holding an OAuth grant is acting as
// the same account a pasted API key would, so it gets the same reach. A grant
// always resolves as ScopeFull — the same reasoning the register endpoint uses
// for a user-created key: an OAuth grant has no narrower variant to request.
func (a apiKeys) AuthenticateAPIKey(ctx context.Context, tokenHash string) (auth.APIKeyIdentity, error) {
	row, err := a.q.AuthenticateAPIKey(ctx, tokenHash)
	if err == nil {
		return auth.APIKeyIdentity{UserID: row.UserID, Scope: row.Scope, KeyID: row.ID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return auth.APIKeyIdentity{}, err
	}
	grant, err := a.q.AuthenticateOAuthGrant(ctx, db.AuthenticateOAuthGrantParams{AccessTokenHash: tokenHash})
	if err != nil {
		return auth.APIKeyIdentity{}, err
	}
	// No endpoint is passed, so the grant's resource binding is not applied: it scopes which MCP endpoint
	// the grant is for, and a REST route is not an MCP server.
	return auth.APIKeyIdentity{UserID: grant.UserID, Scope: auth.ScopeFull, OAuthGrant: true}, nil
}
