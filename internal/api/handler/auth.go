package handler

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/ga4mp"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	appleauth "github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
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

// authHandlers serves the auth surface: register/login/logout/me, OAuth sign-in,
// the browser-extension connect flow, and API-key management. cookieDomains are the
// bare registrable domains we serve (e.g. "freehire.me"): the session cookie is
// scoped to the one the request host falls under (empty = host-only, for dev; see
// auth.CookieDomainForHost). requestOrigin is gated by servedHosts instead.
type authHandlers struct {
	queries       *db.Queries
	issuer        *auth.Issuer
	cookieSecure  bool
	cookieDomains []string
	// oauth is never nil (an empty registry 404s / lists empty).
	oauth oauthRegistry
	// oauthCodes holds the single-use codes that carry a mobile OAuth sign-in to /exchange.
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
	pushNotifier pushnotify.Notifier
	// ga4 sends the GA4 sign_up from the server (me_attribution.go). Nil — and so
	// inert — when GA4_API_SECRET is not configured.
	ga4 ga4mp.Reporter
	// browserOrigins are the origins requireSameOrigin accepts in Origin/Referer (the CORS allowlist's source).
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
	// sameOrigin binds session-issuing routes to the served browser origins: a present
	// Origin or Referer must name one, and Sec-Fetch-Site: cross-site refuses even with
	// both stripped. A fully headerless caller (curl, CLI, mobile SDK) passes. Mounted
	// on the session-COOKIE writers only: a cross-site form post to account-data routes
	// arrives without the cookie (Lax), and API-key callers of /auth/me send no Origin.
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

	// MCP OAuth 2.1. Mounted on api directly (/api/v1/oauth/*, NOT authGroup's
	// /auth/oauth/*, which is social sign-in): the metadata document, the sign-in
	// returnTo and the consent form action are all written against the shorter path.
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

// Login verifies credentials, starts a session, and returns the user. Unknown email,
// wrong password and passwordless accounts all yield the same generic 401.
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

// errAccountSuspended is what every sign-in path answers for a banned account.
var errAccountSuspended = authError(fiber.StatusForbidden, "account_suspended", "This account is suspended")

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

// AuthenticateAPIKey resolves a bearer token hash against api_keys first, then
// oauth_grants. Every guarded REST route accepts either: an MCP client with a grant
// acts as the same account a pasted key would. A grant always resolves as ScopeFull,
// since it has no narrower variant to request.
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
