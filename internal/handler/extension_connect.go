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

// ExtensionConnect renders the consent screen for the browser-extension sign-in.
// Cookie-only (RequireAuth): a leaked API key must not drive it. It validates the
// redirect target before showing anything, so an invalid redirect never reaches a
// consent step.
func (h *authHandlers) ExtensionConnect(c *fiber.Ctx) error {
	if _, err := requireUserID(c); err != nil {
		return err
	}
	redirectURI := c.Query("redirect_uri")
	state := c.Query("state")
	if err := validateExtensionRedirect(redirectURI, h.extensionRedirectAllowlist); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid redirect_uri")
	}
	c.Type("html")
	return c.SendString(consentPage(redirectURI, state))
}

// ExtensionConnectSubmit acts on the consent decision. On approval it mints a
// named API key and 302-redirects the token back in the fragment; on anything
// else it mints nothing and 302-redirects an error. Cookie-only (RequireAuth).
func (h *authHandlers) ExtensionConnectSubmit(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	redirectURI := c.FormValue("redirect_uri")
	state := c.FormValue("state")
	// Re-validate: never trust that the GET consent step ran, and never redirect
	// to an unvetted target.
	if err := validateExtensionRedirect(redirectURI, h.extensionRedirectAllowlist); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid redirect_uri")
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
