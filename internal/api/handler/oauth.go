package handler

import (
	"errors"
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth/oauth"
)

// requestOrigin returns the scheme+host origin to use for this request's OAuth
// redirect URLs and post-login redirect. The request Host is honoured only when it
// matches a configured served host EXACTLY; anything else (dev/localhost, a spoofed
// Host, a subdomain we do not serve) falls back to the canonical frontendOrigin.
//
// The exact match is the point. This used to accept any suffix of a cookie domain,
// which meant a hijacked or third-party-hosted subdomain of freehire.me could steer
// the provider redirect and the post-login redirect. Cookie scope still uses the
// suffix rule — that is what SSO across subdomains needs — but a redirect target is
// not a cookie scope, and conflating them is what produced the finding.
func (h *authHandlers) requestOrigin(c *fiber.Ctx) string {
	host := c.Hostname()
	for _, served := range h.servedHosts {
		if host == served {
			return "https://" + host
		}
	}
	return h.frontendOrigin
}

// needsExplicitServedHosts reports a deployment that answers on several hosts while
// leaving SERVED_HOSTS unset. A configured COOKIE_DOMAIN is that signal: the session
// cookie is only ever scoped to a registrable domain when more than one host shares it.
// The default (the frontend origin's host alone) then silently breaks sign-in on every
// other domain — the state cookie is set on the host the flow started from, while the
// callback is sent to the canonical origin, so the state can never match.
func needsExplicitServedHosts(configured, cookieDomains []string) bool {
	return len(configured) == 0 && len(cookieDomains) > 0
}

// servedHostsOrDefault falls back to the frontend origin's own host when SERVED_HOSTS
// is unset, so a deployment that never sets it keeps working on its canonical domain
// (and every other Host simply gets the canonical origin).
func servedHostsOrDefault(configured []string, frontendOrigin string) []string {
	if len(configured) > 0 {
		return configured
	}
	u, err := url.Parse(frontendOrigin)
	if err != nil || u.Host == "" {
		return nil
	}
	return []string{u.Hostname()}
}

// ListOAuthProviders returns the names of enabled OAuth providers, so the SPA
// renders only usable sign-in buttons.
func (h *authHandlers) ListOAuthProviders(c *fiber.Ctx) error {
	names := h.oauth.Names()
	sort.Strings(names)
	return c.JSON(fiber.Map{"data": names})
}

// OAuthStart begins the authorization-code flow: it stores a fresh CSRF state
// in a short-lived cookie and redirects the browser to the provider's consent
// page carrying the same state. `?platform=mobile` records that the flow was
// started by the native app, so the callback finishes as a deep link.
func (h *authHandlers) OAuthStart(c *fiber.Ctx) error {
	p, ok := h.oauth.Provider(c.Params("provider"), h.requestOrigin(c))
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "unknown provider")
	}

	state, err := oauth.NewState()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to start sign-in")
	}
	oauth.SetStateCookie(c, state, h.cookieSecure)
	// Remember where the SPA wants the user back, so sign-in from a deep page
	// (e.g. a job detail) returns there. Sanitized to a same-origin path.
	oauth.SetReturnCookie(c, oauth.SafeReturnPath(c.Query("returnTo")), h.cookieSecure)
	if c.Query("platform") == oauth.PlatformMobile {
		oauth.SetPlatformCookie(c, oauth.PlatformMobile, h.cookieSecure)
	}
	return c.Redirect(p.AuthCodeURL(state), fiber.StatusFound)
}

// OAuthCallback completes the flow: verify the CSRF state, exchange the code
// for the provider identity, resolve (or create) the account, then finish. On
// the web it starts the session and redirects to the SPA; for a mobile flow it
// mints a one-time code and redirects to the app's custom scheme (the session
// is minted later, by the app's own /exchange call). Every failure redirects
// with auth_error instead of rendering JSON; details go to the server log.
func (h *authHandlers) OAuthCallback(c *fiber.Ctx) error {
	// The callback lands on the same host the flow started on, so this origin
	// matches the redirect_uri sent to the provider (required for the exchange)
	// and is where the browser is sent back afterwards.
	origin := h.requestOrigin(c)
	p, ok := h.oauth.Provider(c.Params("provider"), origin)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "unknown provider")
	}

	// The state, return target, and platform are single-use: clear all three
	// cookies no matter how the rest goes. Re-sanitize the return path.
	cookieState := c.Cookies(oauth.StateCookieName)
	returnTo := oauth.SafeReturnPath(c.Cookies(oauth.ReturnCookieName))
	mobile := c.Cookies(oauth.PlatformCookieName) == oauth.PlatformMobile
	oauth.ClearStateCookie(c, h.cookieSecure)
	oauth.ClearReturnCookie(c, h.cookieSecure)
	oauth.ClearPlatformCookie(c, h.cookieSecure)

	// Apple's callback arrives as a POST with a form-encoded body instead of a
	// GET query string — response_mode=form_post is mandatory once the email
	// scope is requested. Every other provider's callback is still a GET.
	state, code := c.Query("state"), c.Query("code")
	if c.Method() == fiber.MethodPost {
		state, code = c.FormValue("state"), c.FormValue("code")
	}
	if state == "" || state != cookieState {
		return h.oauthFail(c, p.Name(), returnTo, mobile, errors.New("state mismatch"))
	}
	if code == "" {
		return h.oauthFail(c, p.Name(), returnTo, mobile, errors.New("missing code"))
	}

	identity, err := p.FetchIdentity(c.Context(), code)
	if err != nil {
		return h.oauthFail(c, p.Name(), returnTo, mobile, err)
	}

	userID, err := h.accounts.ResolveOAuthAccount(c.Context(), p.Name(), identity.ProviderUserID, identity.Email, identity.EmailVerified)
	if err != nil {
		return h.oauthFail(c, p.Name(), returnTo, mobile, err)
	}

	// The majority sign-up path, and the reason attribution rides in a cookie at all: this
	// is a GET redirect back from the provider, so there is no request body a code could
	// have travelled in. Safe to call for a returning user — the SQL only attributes an
	// account created within the hour.
	h.attributeInvite(c, userID)

	if mobile {
		// Hand the app a single-use code instead of a cookie; the app exchanges
		// it over its own client so the session cookie lands in its jar.
		otc, err := h.oauthCodes.Mint(userID)
		if err != nil {
			return h.oauthFail(c, p.Name(), returnTo, mobile, err)
		}
		return c.Redirect(oauth.MobileCallbackURL+"?code="+url.QueryEscape(otc), fiber.StatusFound)
	}

	if err := h.setSession(c, userID); err != nil {
		return h.oauthFail(c, p.Name(), returnTo, mobile, err)
	}
	return c.Redirect(origin+returnTo, fiber.StatusFound)
}

// OAuthExchange redeems the one-time code from a mobile OAuth callback for a
// session. Because the app makes this request, the session cookie set here
// lands in the app's cookie jar (the whole point of the mobile handshake). A
// missing/expired/reused code is a generic 401.
func (h *authHandlers) OAuthExchange(c *fiber.Ctx) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	userID, ok := h.oauthCodes.Consume(in.Code)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired code")
	}
	if err := h.setSession(c, userID); err != nil {
		return err
	}
	user, err := h.accounts.UserByID(c.Context(), userID)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// oauthFail logs the failure server-side and sends the client back to where
// sign-in started with the generic auth_error marker (never a JSON error page).
// Mobile flows bounce to the app's custom scheme; web flows to the SPA path.
func (h *authHandlers) oauthFail(c *fiber.Ctx, provider, returnTo string, mobile bool, err error) error {
	log.Printf("oauth %s: sign-in failed: %v", provider, err)
	if mobile {
		return c.Redirect(oauth.MobileCallbackURL+"?auth_error=oauth", fiber.StatusFound)
	}
	sep := "?"
	if strings.Contains(returnTo, "?") {
		sep = "&"
	}
	return c.Redirect(h.requestOrigin(c)+returnTo+sep+"auth_error=oauth", fiber.StatusFound)
}
