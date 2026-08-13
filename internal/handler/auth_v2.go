package handler

import (
	"crypto/subtle"
	"errors"
	"net/url"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/mobileauth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/auth/recentauth"
	"github.com/strelov1/freehire/internal/ratelimit"
)

type oauthV2Registry interface {
	ProviderV2(name, origin string) (oauth.Provider, bool)
}

const oauthV2StateCookieName = "hire_oauth_v2_state"

// errOAuthV2StateMismatch and errOAuthV2MissingCode are named so oauthV2Fail
// can tell an ordinary CSRF-state check or a malformed provider callback (both
// routine, not worth a Sentry report) apart from a genuine infra failure.
var (
	errOAuthV2StateMismatch = errors.New("oauth v2: state mismatch")
	errOAuthV2MissingCode   = errors.New("oauth v2: missing code")
)

// isExpectedAttemptFailure reports whether err is one of the mobileauth
// validation sentinels — caller sent a malformed or stale attempt — rather
// than an unexpected infra failure that should reach Sentry via RenderError.
func isExpectedAttemptFailure(err error) bool {
	return errors.Is(err, mobileauth.ErrInvalid) ||
		errors.Is(err, mobileauth.ErrVerifier) ||
		errors.Is(err, mobileauth.ErrSession)
}

// isExpectedOAuthV2Failure extends isExpectedAttemptFailure with the other
// routine causes oauthV2Fail sees: reauth identity mismatch, an account with
// no verified email, and the two local checks in OAuthCallbackV2 itself.
func isExpectedOAuthV2Failure(err error) bool {
	return isExpectedAttemptFailure(err) ||
		errors.Is(err, mobileauth.ErrIdentity) ||
		errors.Is(err, accounts.ErrNoVerifiedEmail) ||
		errors.Is(err, errOAuthV2StateMismatch) ||
		errors.Is(err, errOAuthV2MissingCode)
}

func (h *authHandlers) currentSessionHash(c *fiber.Ctx) ([]byte, error) {
	raw := c.Cookies(auth.CookieName)
	if raw == "" {
		return nil, authError(401, "unauthorized", "unauthorized")
	}
	return h.issuer.SessionFingerprint(raw)
}

func (h *authHandlers) currentSessionHashOrRotate(c *fiber.Ctx, userID int64, version int32) ([]byte, error) {
	fingerprint, err := h.currentSessionHash(c)
	if err == nil {
		return fingerprint, nil
	}
	if !errors.Is(err, auth.ErrNoSessionID) {
		return nil, err
	}
	raw, err := h.issueSessionAt(c, userID, version)
	if err != nil {
		return nil, err
	}
	return h.issuer.SessionFingerprint(raw)
}

func (h *authHandlers) sessionMatches(expected []byte, c *fiber.Ctx) bool {
	actual, err := h.currentSessionHash(c)
	return err == nil && len(expected) == len(actual) && subtle.ConstantTimeCompare(expected, actual) == 1
}

func (h *authHandlers) registerV2(api fiber.Router, mw middleware) {
	noStore := func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		c.Set("Pragma", "no-cache")
		return c.Next()
	}
	g := api.Group("/auth", noStore)
	exchangeLimit := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("oauth-v2-exchange"), 20, time.Minute)
	passwordIPLimit := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("password-reauth-ip"), 20, time.Minute)
	passwordUserLimit := ratelimit.Middleware(h.throttler, ratelimit.KeyByUserOrIP("password-reauth-user"), 5, time.Minute)
	// The same-repository web bundle calls password reauthentication before
	// protected mutations even during a schema-first rollout where mobile v2 is
	// still disabled. Keeping this one issuance route available prevents that
	// compatibility deploy from breaking password and API-key management.
	g.Post("/reauth/password", passwordIPLimit, mw.cookie, passwordUserLimit, h.PasswordReauthV2)
	if !h.authV2Enabled {
		return
	}
	g.Get("/providers", h.ListProvidersV2)
	startLimit := ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("oauth-v2-start"), 20, time.Minute)
	g.Get("/oauth/:provider/start", startLimit, mw.optionalCookie, h.OAuthStartV2)
	g.Get("/oauth/:provider/callback", h.OAuthCallbackV2)
	g.Post("/oauth/:provider/callback", h.OAuthCallbackV2)
	g.Post("/oauth/exchange", exchangeLimit, mw.optionalCookie, h.OAuthExchangeV2)
	g.Post("/apple/attempt", startLimit, mw.optionalCookie, h.AppleAttemptV2)
	g.Post("/apple/exchange", exchangeLimit, mw.optionalCookie, h.AppleExchangeV2)
	g.Get("/identities", mw.cookie, h.ListIdentitiesV2)
	g.Delete("/identities/:provider", mw.cookie, h.UnlinkIdentityV2)
}

func (h *authHandlers) ListProvidersV2(c *fiber.Ctx) error {
	type provider struct {
		ID        string   `json:"id"`
		Flow      string   `json:"flow"`
		Platforms []string `json:"platforms"`
		Available bool     `json:"available"`
	}
	var providers []provider
	platforms := make([]string, 0, 2)
	for _, platform := range []string{"ios", "android"} {
		if h.mobileCallbacks[platform] != "" {
			platforms = append(platforms, platform)
		}
	}
	for _, name := range h.oauth.Names() {
		if name == "apple" || len(platforms) == 0 {
			continue
		}
		providers = append(providers, provider{name, "browser_oauth", append([]string(nil), platforms...), true})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	if h.appleNative != nil && h.appleGrantKeys != nil {
		providers = append(providers, provider{"apple", "native_apple", []string{"ios"}, true})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"schema_version": 2, "providers": providers}})
}

func (h *authHandlers) currentBinding(c *fiber.Ctx, purpose string) (*mobileauth.Binding, error) {
	if purpose != "reauth" {
		if purpose != "sign_in" {
			return nil, authError(400, "invalid_purpose", "invalid purpose")
		}
		return nil, nil
	}
	uid, err := requireUserID(c)
	if err != nil {
		return nil, authError(401, "unauthorized", "unauthorized")
	}
	version, err := h.queries.GetUserTokenVersion(c.Context(), uid)
	if err != nil {
		return nil, err
	}
	sessionHash, err := h.currentSessionHashOrRotate(c, uid, version)
	if err != nil {
		return nil, err
	}
	return &mobileauth.Binding{UserID: uid, TokenVersion: version, SessionHash: sessionHash}, nil
}

func (h *authHandlers) OAuthStartV2(c *fiber.Ctx) error {
	registry, ok := h.oauth.(oauthV2Registry)
	if !ok {
		return authError(503, "oauth_unavailable", "sign-in unavailable")
	}
	provider := c.Params("provider")
	p, ok := registry.ProviderV2(provider, h.requestOrigin(c))
	if !ok {
		return authError(404, "unknown_provider", "unknown provider")
	}
	platform, target, purpose := c.Query("platform"), c.Query("callback_target"), c.Query("purpose")
	if provider == "apple" && (platform != "web" || purpose != "reauth") {
		return authError(404, "unknown_provider", "unknown provider")
	}
	callback, ok := h.mobileCallbacks[target]
	if !ok || callback == "" || target != platform {
		return authError(400, "invalid_callback_target", "invalid callback target")
	}
	_ = callback
	if c.Query("code_challenge_method") != "S256" {
		return authError(400, "invalid_code_challenge", "S256 is required")
	}
	binding, err := h.currentBinding(c, purpose)
	if err != nil {
		return err
	}
	_, state, err := h.mobileAuth.CreateBrowserAttempt(c.Context(), provider, platform, target, purpose, c.Query("code_challenge"), binding)
	if err != nil {
		if isExpectedAttemptFailure(err) {
			return authError(400, "invalid_oauth_attempt", "invalid OAuth attempt")
		}
		return err
	}
	oauth.SetStateCookieNamed(c, oauthV2StateCookieName, state, h.cookieSecure)
	return c.Redirect(p.AuthCodeURL(state), fiber.StatusFound)
}

func callbackWith(raw, key, value string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (h *authHandlers) OAuthCallbackV2(c *fiber.Ctx) error {
	registry, ok := h.oauth.(oauthV2Registry)
	if !ok {
		return authError(503, "oauth_unavailable", "sign-in unavailable")
	}
	provider := c.Params("provider")
	p, ok := registry.ProviderV2(provider, h.requestOrigin(c))
	if !ok {
		return authError(404, "unknown_provider", "unknown provider")
	}
	state, code := c.Query("state"), c.Query("code")
	if c.Method() == fiber.MethodPost {
		state, code = c.FormValue("state"), c.FormValue("code")
	}
	cookieState := c.Cookies(oauthV2StateCookieName)
	oauth.ClearStateCookieNamed(c, oauthV2StateCookieName, h.cookieSecure)
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return h.oauthV2Fail(c, "", errOAuthV2StateMismatch)
	}
	attempt, err := h.mobileAuth.ConsumeBrowserAttempt(c.Context(), state, provider)
	if err != nil {
		return h.oauthV2Fail(c, "", err)
	}
	if code == "" {
		return h.oauthV2Fail(c, attempt.CallbackTarget, errOAuthV2MissingCode)
	}
	identity, err := p.FetchIdentity(c.Context(), code)
	if err != nil {
		return h.oauthV2Fail(c, attempt.CallbackTarget, err)
	}
	var userID int64
	if attempt.Purpose == "reauth" {
		userID, err = h.mobileAuth.IdentityOwner(c.Context(), provider, identity.ProviderUserID)
		if err == nil {
			err = attempt.CheckExpectedUser(userID)
		}
	} else {
		userID, err = h.accounts.ResolveOAuthAccount(c.Context(), provider, identity.ProviderUserID, identity.Email, identity.EmailVerified)
	}
	if err != nil {
		return h.oauthV2Fail(c, attempt.CallbackTarget, err)
	}
	otc, err := h.mobileAuth.MintCode(c.Context(), attempt, userID)
	if err != nil {
		return h.oauthV2Fail(c, attempt.CallbackTarget, err)
	}
	target, err := callbackWith(h.mobileCallbacks[attempt.CallbackTarget], "code", otc)
	if err != nil {
		return err
	}
	return c.Redirect(target, fiber.StatusFound)
}

func (h *authHandlers) oauthV2Fail(c *fiber.Ctx, target string, cause error) error {
	code := "oauth_failed"
	if errors.Is(cause, mobileauth.ErrIdentity) {
		code = "reauth_identity_mismatch"
	}
	// This leg always redirects rather than returning through RenderError, so
	// it has to open its own Sentry gate: a routine cause (bad state, expired
	// attempt, provider rejected the code) stays silent, anything else — a DB
	// error out of the store, the OAuth provider itself being down — is a
	// genuine fault and must not vanish behind a generic "sign-in failed".
	if !isExpectedOAuthV2Failure(cause) {
		reportUnexpected(c, cause)
	}
	base := h.mobileCallbacks[target]
	if base == "" {
		return authError(401, code, "sign-in failed")
	}
	u, err := callbackWith(base, "auth_error", code)
	if err != nil {
		return err
	}
	return c.Redirect(u, fiber.StatusFound)
}

func (h *authHandlers) issueRecent(c *fiber.Ctx, userID int64, version int32, sessionHash []byte) error {
	raw, expires, err := h.recentAuth.Issue(c.Context(), userID, version, sessionHash)
	if err != nil {
		return err
	}
	recentauth.SetCookie(c, raw, expires, h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	return c.JSON(fiber.Map{"data": fiber.Map{"recent_auth_expires_at": expires}})
}

func (h *authHandlers) OAuthExchangeV2(c *fiber.Ctx) error {
	var in struct {
		Code     string `json:"code"`
		Verifier string `json:"code_verifier"`
	}
	if err := c.BodyParser(&in); err != nil {
		return authError(400, "invalid_request", "invalid request body")
	}
	ex, err := h.mobileAuth.ConsumeCode(c.Context(), in.Code, in.Verifier)
	if err != nil {
		if errors.Is(err, mobileauth.ErrInvalid) || errors.Is(err, mobileauth.ErrVerifier) || errors.Is(err, mobileauth.ErrIdentity) {
			return authError(401, "invalid_exchange_code", "invalid or expired code")
		}
		return err
	}
	if ex.Purpose == "reauth" {
		current, currentErr := requireUserID(c)
		if currentErr != nil || current != ex.UserID || !h.sessionMatches(ex.ExpectedSessionHash, c) {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
		if ex.ExpectedTokenVersion == nil {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
		if err = h.mobileAuth.VerifyBinding(c.Context(), ex.UserID, *ex.ExpectedTokenVersion); err != nil {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
		return h.issueRecent(c, ex.UserID, *ex.ExpectedTokenVersion, ex.ExpectedSessionHash)
	}
	user, err := h.accounts.UserByID(c.Context(), ex.UserID)
	if err != nil {
		return accountsError(err)
	}
	if err = h.setSession(c, ex.UserID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

func (h *authHandlers) AppleAttemptV2(c *fiber.Ctx) error {
	if h.appleNative == nil || h.appleGrantKeys == nil {
		return authError(503, "apple_unavailable", "Apple sign-in unavailable")
	}
	var in struct {
		Purpose        string `json:"purpose"`
		NonceChallenge string `json:"nonce_challenge"`
	}
	if err := c.BodyParser(&in); err != nil {
		return authError(400, "invalid_request", "invalid request body")
	}
	binding, err := h.currentBinding(c, in.Purpose)
	if err != nil {
		return err
	}
	a, err := h.mobileAuth.CreateAppleAttempt(c.Context(), in.NonceChallenge, in.Purpose, h.appleNative.ClientIDs(), binding)
	if err != nil {
		if isExpectedAttemptFailure(err) {
			return authError(400, "invalid_apple_attempt", "invalid Apple attempt")
		}
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"attempt_id": a.ID, "expires_at": a.ExpiresAt}})
}

func (h *authHandlers) AppleExchangeV2(c *fiber.Ctx) error {
	if h.appleNative == nil || h.appleGrantKeys == nil {
		return authError(503, "apple_unavailable", "Apple sign-in unavailable")
	}
	var in struct {
		AttemptID         string `json:"attempt_id"`
		IdentityToken     string `json:"identity_token"`
		AuthorizationCode string `json:"authorization_code"`
		RawNonce          string `json:"raw_nonce"`
	}
	if err := c.BodyParser(&in); err != nil {
		return authError(400, "invalid_request", "invalid request body")
	}
	id, err := uuid.Parse(in.AttemptID)
	if err != nil {
		return authError(401, "invalid_apple_attempt", "invalid or expired Apple attempt")
	}
	attempt, err := h.mobileAuth.ConsumeAppleAttempt(c.Context(), id)
	if err != nil {
		if errors.Is(err, mobileauth.ErrInvalid) {
			return authError(401, "invalid_apple_attempt", "invalid or expired Apple attempt")
		}
		return err
	}
	if mobileauth.ValidateVerifier(in.RawNonce) != nil || subtle.ConstantTimeCompare([]byte(mobileauth.Challenge(in.RawNonce)), []byte(attempt.NonceChallenge)) != 1 {
		return authError(401, "invalid_apple_nonce", "Apple nonce mismatch")
	}
	if attempt.Purpose == "reauth" && !h.sessionMatches(attempt.ExpectedSessionHash, c) {
		return authError(401, "reauth_session_mismatch", "session changed")
	}
	claims, err := h.appleNative.Verify(c.Context(), in.IdentityToken, attempt.NonceChallenge)
	if err != nil {
		return authError(401, "invalid_apple_token", "invalid Apple identity token")
	}
	clientID, ok := h.appleNative.ClientIDForClaims(claims)
	if !ok {
		return authError(401, "invalid_apple_token", "invalid Apple identity token")
	}
	attemptAudience := false
	for _, allowed := range attempt.AllowedAudiences {
		if allowed == clientID {
			attemptAudience = true
			break
		}
	}
	if !attemptAudience {
		return authError(401, "invalid_apple_token", "invalid Apple identity token")
	}
	email, verified := claims.VerifiedEmail()
	var userID int64
	if attempt.Purpose == "reauth" {
		current, currentErr := requireUserID(c)
		if currentErr != nil || attempt.ExpectedUserID == nil || current != *attempt.ExpectedUserID {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
		userID, err = h.mobileAuth.IdentityOwner(c.Context(), "apple", claims.Subject)
		if err == nil {
			err = attempt.CheckExpectedUser(userID)
		}
	} else {
		userID, err = h.accounts.ResolveOAuthAccount(c.Context(), "apple", claims.Subject, email, verified)
	}
	if errors.Is(err, accounts.ErrNoVerifiedEmail) {
		return authError(409, "apple_email_unavailable", "Apple account needs support recovery")
	}
	if err != nil {
		if attempt.Purpose == "reauth" {
			return authError(401, "reauth_identity_mismatch", "Apple identity does not match")
		}
		return authError(401, "apple_identity_mismatch", "Apple identity does not match")
	}
	if attempt.Purpose == "reauth" && attempt.ExpectedTokenVersion != nil {
		if err = h.mobileAuth.VerifyBinding(c.Context(), userID, *attempt.ExpectedTokenVersion); err != nil {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
	}
	grant, err := h.appleNative.Exchange(c.Context(), in.AuthorizationCode, clientID, attempt.NonceChallenge, claims.Subject)
	if err != nil {
		return authError(401, "apple_exchange_failed", "Apple sign-in failed")
	}
	compensationID, err := h.mobileAuth.CreateAppleCompensation(c.Context(), h.appleGrantKeys, claims.Subject, clientID, grant.RefreshToken)
	if err != nil {
		// If durable custody is unavailable, revoke while the only plaintext copy
		// is still in memory. Joining errors preserves the operational failure
		// without ever including the token value.
		if revokeErr := h.appleNative.Revoke(c.Context(), grant.RefreshToken, clientID); revokeErr != nil {
			return errors.Join(err, revokeErr)
		}
		return err
	}
	if err = h.mobileAuth.FinalizeAppleGrant(c.Context(), userID, claims.Subject, clientID, compensationID); err != nil {
		// The delayed job already owns an encrypted copy. Make it immediately
		// eligible when possible; a failed activation still leaves the durable
		// delayed job for crash recovery.
		_ = h.mobileAuth.ActivateAppleCompensation(c.Context(), compensationID)
		if errors.Is(err, mobileauth.ErrState) || errors.Is(err, mobileauth.ErrIdentity) {
			return authError(409, "apple_identity_unavailable", "Apple identity is unavailable")
		}
		return err
	}
	if err = h.mobileAuth.DisarmAppleCompensation(c.Context(), compensationID); err != nil {
		// Do not issue a session while the compensation job can still revoke the
		// newly stored grant. A retry creates a new grant and safely supersedes it.
		return err
	}
	if attempt.Purpose == "reauth" {
		if attempt.ExpectedTokenVersion == nil {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
		if err = h.mobileAuth.VerifyBinding(c.Context(), userID, *attempt.ExpectedTokenVersion); err != nil {
			return authError(401, "reauth_session_mismatch", "session changed")
		}
		return h.issueRecent(c, userID, *attempt.ExpectedTokenVersion, attempt.ExpectedSessionHash)
	}
	user, err := h.accounts.UserByID(c.Context(), userID)
	if err != nil {
		return accountsError(err)
	}
	if err = h.setSession(c, userID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

func (h *authHandlers) PasswordReauthV2(c *fiber.Ctx) error {
	uid, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in struct {
		Password string `json:"password"`
	}
	if err = c.BodyParser(&in); err != nil {
		return authError(400, "invalid_request", "invalid request body")
	}
	user, err := h.accounts.UserByID(c.Context(), uid)
	if err != nil {
		return err
	}
	authenticated, err := h.accounts.Login(c.Context(), user.Email, in.Password)
	if errors.Is(err, accounts.ErrInvalidCredentials) || (err == nil && authenticated.ID != uid) {
		return authError(401, "invalid_credentials", "invalid credentials")
	}
	if err != nil {
		return accountsError(err)
	}
	version, err := h.queries.GetUserTokenVersion(c.Context(), uid)
	if err != nil {
		return err
	}
	sessionHash, err := h.currentSessionHashOrRotate(c, uid, version)
	if err != nil {
		return err
	}
	return h.issueRecent(c, uid, version, sessionHash)
}

func (h *authHandlers) ListIdentitiesV2(c *fiber.Ctx) error {
	uid, err := requireUserID(c)
	if err != nil {
		return err
	}
	out, err := h.mobileAuth.ListIdentities(c.Context(), uid)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": out})
}

func (h *authHandlers) UnlinkIdentityV2(c *fiber.Ctx) error {
	uid, err := requireUserID(c)
	if err != nil {
		return err
	}
	version, err := h.queries.GetUserTokenVersion(c.Context(), uid)
	if err != nil {
		return err
	}
	sessionHash, sessionErr := h.currentSessionHash(c)
	if sessionErr != nil {
		return sessionErr
	}
	pending, err := h.mobileAuth.UnlinkIdentity(c.Context(), uid, version, sessionHash, c.Params("provider"), c.Cookies(recentauth.CookieName))
	switch {
	case errors.Is(err, mobileauth.ErrSession):
		return authError(428, "recent_auth_required", "recent authentication required")
	case errors.Is(err, mobileauth.ErrLastMethod):
		return authError(409, "last_sign_in_method", "cannot remove the last sign-in method")
	case errors.Is(err, mobileauth.ErrState):
		return authError(409, "identity_state_conflict", "identity state conflict")
	case errors.Is(err, pgx.ErrNoRows):
		return authError(404, "identity_not_found", "identity not found")
	case err != nil:
		return err
	}
	recentauth.ClearCookie(c, h.cookieSecure, auth.CookieDomainForHost(c.Hostname(), h.cookieDomains))
	if pending {
		return c.Status(202).JSON(fiber.Map{"data": fiber.Map{"status": "revocation_pending"}})
	}
	return c.SendStatus(204)
}
