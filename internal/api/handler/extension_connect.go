package handler

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// extensionRedirectSuffixes are the host suffixes a browser's identity redirect
// URL carries: launchWebAuthFlow only resolves a redirect to its own browser's
// domain — https://<extension-id>.chromiumapp.org in Chrome,
// https://<sha1-of-addon-id>.extensions.allizom.org in Firefox. Both domains belong
// to the browser vendor, so a label under either names one installed extension.
var extensionRedirectSuffixes = []string{".chromiumapp.org", ".extensions.allizom.org"}

// validateExtensionRedirect reports whether redirectURI is a safe target for the
// browser-extension connect flow to hand a minted token to. It accepts only an
// https URL on one of extensionRedirectSuffixes whose leading part is a single
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
	for _, suffix := range extensionRedirectSuffixes {
		id, ok := strings.CutSuffix(host, suffix)
		if !ok {
			continue
		}
		if id == "" || strings.Contains(id, ".") {
			return fmt.Errorf("redirect_uri must be a single <id>%s label, got %q", suffix, host)
		}
		if !slices.Contains(allowlist, id) {
			return fmt.Errorf("extension id %q is not allowlisted", id)
		}
		return nil
	}
	return fmt.Errorf("redirect_uri host must be under %v, got %q", extensionRedirectSuffixes, host)
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
	userID, err := requireUserID(c)
	if err != nil {
		// `via=web` marks a request the web app sent back after signing the visitor in.
		// It is the loop stop: without it, a session the browsing profile has but this
		// window cannot see would bounce between the two forever.
		if c.Query("via") == "web" {
			page, err := noSessionPage(h.frontendOrigin)
			if err != nil {
				return fmt.Errorf("render the extension no-session page: %w", err)
			}
			c.Type("html")
			return c.Status(fiber.StatusUnauthorized).SendString(page)
		}
		return c.Redirect(h.signInURL(redirectURI, state), fiber.StatusFound)
	}
	user, err := h.queries.GetUserByID(c.Context(), userID)
	if err != nil {
		return fmt.Errorf("read the account behind the extension connect of user %d: %w", userID, err)
	}
	page, err := consentPage(redirectURI, state, user.Email, h.frontendOrigin)
	if err != nil {
		return fmt.Errorf("render the extension consent for user %d: %w", userID, err)
	}
	c.Type("html")
	return c.SendString(page)
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
	if c.FormValue("decision") == decisionSwitch {
		return h.switchAccount(c, h.signInURL(redirectURI, state))
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
	//
	// Both failures are returned rather than phrased as fiber.NewError(500, …): what
	// has to read them is the error inbox, and a *fiber.Error carries no cause for it
	// to show. The caller meets the generic 500 body instead — see classify. The read
	// additionally gains the house mapping for a vanished account (pgx.ErrNoRows →
	// 404), which is the honest answer when the session outlives its user row.
	version, err := h.queries.GetUserTokenVersion(c.Context(), userID)
	if err != nil {
		return fmt.Errorf("read token version for the extension connect of user %d: %w", userID, err)
	}
	token, err := h.issuer.Issue(userID, version)
	if err != nil {
		return fmt.Errorf("issue extension connect token for user %d: %w", userID, err)
	}
	return redirect(url.Values{"token": {token}, "state": {state}})
}

// noSessionPage is the dead end of the sign-in round trip: the visitor came back from
// the web app and this window still has no session. It says what to do instead of
// leaving Chrome to report "Authorization page could not be loaded".
func noSessionPage(origin string) (string, error) {
	return renderNoticeScreen(noticeScreen{
		Title:     "Not signed in",
		Message:   "This window could not pick up a freehire session. Sign in on the website, then press Sign in with freehire in the extension again.",
		LinkURL:   strings.TrimSuffix(origin, "/") + "/signin",
		LinkLabel: "Sign in to freehire",
	})
}

// extensionAllows adds the one capability only the extension's own connection
// has: auth.IsExtensionBearer confines the assistant's page tools and autofill
// to it.
var extensionAllows = append(slices.Clone(bearerAllows), "Read and fill in the web pages you ask it to work on.")

// consentPage is the browser extension's approval screen. The redirect and state
// ride hidden fields back to the POST.
func consentPage(redirectURI, state, email, origin string) (string, error) {
	return renderConsentScreen(consentScreen{
		Title:    "The freehire extension wants to access your account",
		SignedIn: email,
		Allows:   extensionAllows,
		Denies:   bearerDenies,
		Caution:  "The extension acts as you. Allow only if you pressed Sign in with freehire in it yourself.",
		Action:   "/api/v1/auth/extension/connect",
		Hidden:   []hiddenField{{"redirect_uri", redirectURI}, {"state", state}},
		Origin:   origin,
	})
}
