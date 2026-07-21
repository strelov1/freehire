package handler

import (
	"errors"
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth/oauth"
)

// ListOAuthProviders returns the names of enabled OAuth providers, so the SPA
// renders only usable sign-in buttons.
func (a *API) ListOAuthProviders(c *fiber.Ctx) error {
	names := make([]string, 0, len(a.oauth))
	for name := range a.oauth {
		names = append(names, name)
	}
	sort.Strings(names)
	return c.JSON(fiber.Map{"data": names})
}

// OAuthStart begins the authorization-code flow: it stores a fresh CSRF state
// in a short-lived cookie and redirects the browser to the provider's consent
// page carrying the same state. `?platform=mobile` records that the flow was
// started by the native app, so the callback finishes as a deep link.
func (a *API) OAuthStart(c *fiber.Ctx) error {
	p, ok := a.oauth[c.Params("provider")]
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "unknown provider")
	}

	state, err := oauth.NewState()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to start sign-in")
	}
	oauth.SetStateCookie(c, state, a.cookieSecure)
	// Remember where the SPA wants the user back, so sign-in from a deep page
	// (e.g. a job detail) returns there. Sanitized to a same-origin path.
	oauth.SetReturnCookie(c, oauth.SafeReturnPath(c.Query("returnTo")), a.cookieSecure)
	if c.Query("platform") == oauth.PlatformMobile {
		oauth.SetPlatformCookie(c, oauth.PlatformMobile, a.cookieSecure)
	}
	return c.Redirect(p.AuthCodeURL(state), fiber.StatusFound)
}

// OAuthCallback completes the flow: verify the CSRF state, exchange the code
// for the provider identity, resolve (or create) the account, then finish. On
// the web it starts the session and redirects to the SPA; for a mobile flow it
// mints a one-time code and redirects to the app's custom scheme (the session
// is minted later, by the app's own /exchange call). Every failure redirects
// with auth_error instead of rendering JSON; details go to the server log.
func (a *API) OAuthCallback(c *fiber.Ctx) error {
	p, ok := a.oauth[c.Params("provider")]
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "unknown provider")
	}

	// The state, return target, and platform are single-use: clear all three
	// cookies no matter how the rest goes. Re-sanitize the return path.
	cookieState := c.Cookies(oauth.StateCookieName)
	returnTo := oauth.SafeReturnPath(c.Cookies(oauth.ReturnCookieName))
	mobile := c.Cookies(oauth.PlatformCookieName) == oauth.PlatformMobile
	oauth.ClearStateCookie(c, a.cookieSecure)
	oauth.ClearReturnCookie(c, a.cookieSecure)
	oauth.ClearPlatformCookie(c, a.cookieSecure)

	state, code := c.Query("state"), c.Query("code")
	if state == "" || state != cookieState {
		return a.oauthFail(c, p.Name(), returnTo, mobile, errors.New("state mismatch"))
	}
	if code == "" {
		return a.oauthFail(c, p.Name(), returnTo, mobile, errors.New("missing code"))
	}

	identity, err := p.FetchIdentity(c.Context(), code)
	if err != nil {
		return a.oauthFail(c, p.Name(), returnTo, mobile, err)
	}

	userID, err := a.accounts.ResolveOAuthAccount(c.Context(), p.Name(), identity.ProviderUserID, identity.Email, identity.EmailVerified)
	if err != nil {
		return a.oauthFail(c, p.Name(), returnTo, mobile, err)
	}

	if mobile {
		// Hand the app a single-use code instead of a cookie; the app exchanges
		// it over its own client so the session cookie lands in its jar.
		otc, err := a.oauthCodes.Mint(userID)
		if err != nil {
			return a.oauthFail(c, p.Name(), returnTo, mobile, err)
		}
		return c.Redirect(oauth.MobileCallbackURL+"?code="+url.QueryEscape(otc), fiber.StatusFound)
	}

	if err := a.setSession(c, userID); err != nil {
		return a.oauthFail(c, p.Name(), returnTo, mobile, err)
	}
	return c.Redirect(a.frontendOrigin+returnTo, fiber.StatusFound)
}

// OAuthExchange redeems the one-time code from a mobile OAuth callback for a
// session. Because the app makes this request, the session cookie set here
// lands in the app's cookie jar (the whole point of the mobile handshake). A
// missing/expired/reused code is a generic 401.
func (a *API) OAuthExchange(c *fiber.Ctx) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	userID, ok := a.oauthCodes.Consume(in.Code)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired code")
	}
	if err := a.setSession(c, userID); err != nil {
		return err
	}
	user, err := a.accounts.UserByID(c.Context(), userID)
	if err != nil {
		return accountsError(err)
	}
	return c.JSON(fiber.Map{"data": toUserResponse(user)})
}

// oauthFail logs the failure server-side and sends the client back to where
// sign-in started with the generic auth_error marker (never a JSON error page).
// Mobile flows bounce to the app's custom scheme; web flows to the SPA path.
func (a *API) oauthFail(c *fiber.Ctx, provider, returnTo string, mobile bool, err error) error {
	log.Printf("oauth %s: sign-in failed: %v", provider, err)
	if mobile {
		return c.Redirect(oauth.MobileCallbackURL+"?auth_error=oauth", fiber.StatusFound)
	}
	sep := "?"
	if strings.Contains(returnTo, "?") {
		sep = "&"
	}
	return c.Redirect(a.frontendOrigin+returnTo+sep+"auth_error=oauth", fiber.StatusFound)
}
