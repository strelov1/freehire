package handler

import (
	"fmt"
	"html"
	"net/url"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// chromiumappSuffix is the host suffix Chrome's identity redirect URLs carry:
// launchWebAuthFlow only resolves a redirect to https://<extension-id>.chromiumapp.org.
const chromiumappSuffix = ".chromiumapp.org"

// validateExtensionRedirect reports whether redirectURI is a safe target for the
// browser-extension connect flow to hand a minted token to. It accepts only an
// https://<extension-id>.chromiumapp.org URL whose <extension-id> is a single
// host label present in allowlist. An empty allowlist disables the flow entirely.
func validateExtensionRedirect(redirectURI string, allowlist []string) error {
	if len(allowlist) == 0 {
		return fmt.Errorf("extension connect is disabled (empty allowlist)")
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		return fmt.Errorf("invalid redirect_uri: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("redirect_uri must be https, got %q", u.Scheme)
	}
	if u.Port() != "" {
		return fmt.Errorf("redirect_uri must not carry a port")
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, chromiumappSuffix) {
		return fmt.Errorf("redirect_uri host must be *%s, got %q", chromiumappSuffix, host)
	}
	id := strings.TrimSuffix(host, chromiumappSuffix)
	if id == "" || strings.Contains(id, ".") {
		return fmt.Errorf("redirect_uri must be a single <id>%s label, got %q", chromiumappSuffix, host)
	}
	if !slices.Contains(allowlist, id) {
		return fmt.Errorf("extension id %q is not allowlisted", id)
	}
	return nil
}

// redirectWithFragment returns base with vals encoded into its URL fragment,
// replacing any existing fragment. The connect flow carries the minted token in
// the fragment (never the query) so it is not sent to servers, logged, or leaked
// via Referer.
func redirectWithFragment(base string, vals url.Values) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Fragment = ""
	return u.String() + "#" + vals.Encode(), nil
}

// signInURL is where an unauthenticated connect request is sent: the web app's own
// /extension/connect page, which signs the visitor in and returns them here. The
// extension's parameters ride along verbatim so nothing is lost across the round trip.
func (h *authHandlers) signInURL(redirectURI, state string) string {
	q := url.Values{"redirect_uri": {redirectURI}, "state": {state}}
	return strings.TrimSuffix(h.frontendOrigin, "/") + "/extension/connect?" + q.Encode()
}

// ExtensionConnect renders the consent screen for the browser-extension sign-in.
// Cookie-only: a leaked API key must not drive it. It validates the redirect target
// before showing anything, so an invalid redirect never reaches a consent step — nor
// gets carried through a sign-in round trip.
//
// A signed-out visitor is sent to sign in rather than refused. The extension opens
// this in Chrome's auth window, whose cookie jar is not the browsing profile's, so
// arriving without a session is the NORMAL first run — and a 401 body there is what
// Chrome reports as "Authorization page could not be loaded", with no way forward.
func (h *authHandlers) ExtensionConnect(c *fiber.Ctx) error {
	redirectURI := c.Query("redirect_uri")
	state := c.Query("state")
	if err := validateExtensionRedirect(redirectURI, h.extensionRedirectAllowlist); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid redirect_uri")
	}
	if _, err := requireUserID(c); err != nil {
		// `via=web` marks a request the web app sent back after signing the visitor in.
		// It is the loop stop: without it, a session the browsing profile has but this
		// window cannot see would bounce between the two forever.
		if c.Query("via") == "web" {
			c.Type("html")
			return c.Status(fiber.StatusUnauthorized).SendString(noSessionPage(h.frontendOrigin))
		}
		return c.Redirect(h.signInURL(redirectURI, state), fiber.StatusFound)
	}
	c.Type("html")
	return c.SendString(consentPage(redirectURI, state))
}

// ExtensionConnectSubmit acts on the consent decision. On approval it issues a
// session JWT (Issuer.Issue) and 302-redirects the token back in the fragment;
// on anything else it issues nothing and 302-redirects an error. Cookie-only (RequireAuth).
func (h *authHandlers) ExtensionConnectSubmit(c *fiber.Ctx) error {
	redirectURI := c.FormValue("redirect_uri")
	state := c.FormValue("state")
	// Re-validate: never trust that the GET consent step ran, and never redirect
	// to an unvetted target.
	if err := validateExtensionRedirect(redirectURI, h.extensionRedirectAllowlist); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid redirect_uri")
	}
	userID, err := requireUserID(c)
	if err != nil {
		// The session can lapse between the consent screen and the decision. Restart the
		// flow rather than render JSON into the auth window — the sign-in page brings the
		// visitor straight back to this same consent.
		return c.Redirect(h.signInURL(redirectURI, state), fiber.StatusFound)
	}
	redirect := func(vals url.Values) error {
		location, err := redirectWithFragment(redirectURI, vals)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid redirect_uri")
		}
		return c.Redirect(location, fiber.StatusFound)
	}

	if c.FormValue("decision") != "allow" {
		return redirect(url.Values{"error": {"access_denied"}, "state": {state}})
	}

	// Unify on the session JWT: hire and the agent (Roy) both verify it with the
	// shared HS256 secret, so one token authenticates everywhere (hire via
	// Authorization: Bearer, Roy via cookie/WS-subprotocol). The token carries the
	// account's session generation, so "sign out everywhere" evicts the extension too —
	// re-running connect mints a fresh one.
	version, err := h.queries.GetUserTokenVersion(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue token")
	}
	token, err := h.issuer.Issue(userID, version)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue token")
	}
	return redirect(url.Values{"token": {token}, "state": {state}})
}

// noSessionPage is the dead end of the sign-in round trip: the visitor came back from
// the web app and this window still has no session. It says what to do instead of
// leaving Chrome to report "Authorization page could not be loaded".
func noSessionPage(origin string) string {
	home := strings.TrimSuffix(origin, "/")
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Sign in to connect the extension</title></head>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">
  <h1>Not signed in</h1>
  <p>This window could not pick up a freehire session. Sign in at
     <a href="%s">%s</a>, then press <b>Sign in with freehire</b> in the extension again.</p>
</body>
</html>`, html.EscapeString(home), html.EscapeString(home))
}

// consentPage is the minimal server-rendered approval screen. The redirect and
// state ride hidden fields back to the POST; both are HTML-escaped into attribute
// values to keep them inert.
func consentPage(redirectURI, state string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Connect the freehire extension</title></head>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">
  <h1>Connect the freehire extension</h1>
  <p>Allow the freehire browser extension to access your account? It will act as
     you — reading your profile, CV, and saved jobs.</p>
  <form method="post" action="/api/v1/auth/extension/connect">
    <input type="hidden" name="redirect_uri" value="%s">
    <input type="hidden" name="state" value="%s">
    <button type="submit" name="decision" value="allow">Allow</button>
    <button type="submit" name="decision" value="cancel">Cancel</button>
  </form>
</body>
</html>`, html.EscapeString(redirectURI), html.EscapeString(state))
}
