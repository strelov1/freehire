package handler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// authorizationCodeTTL is how long a code is redeemable before the token
// exchange must have happened. Ten minutes matches the window other OAuth
// servers use for a human-paced consent click, not a machine-paced retry.
const authorizationCodeTTL = 10 * time.Minute

// authorizeParams is the subset of RFC 6749/PKCE query params this server reads.
// response_type is required to be "code" and code_challenge_method required to
// be "S256" — anything else is refused before a client row is even looked up.
// state is optional per RFC 6749, though every caller is expected to send one.
type authorizeParams struct {
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	// Resource is the RFC 8707 MCP resource the grant will be bound to, already
	// canonical, or "" for an unbound grant. Set by resolveAuthorizeResource.
	Resource string
}

func parseAuthorizeParams(responseType, clientID, redirectURI, state, challenge, method string) (authorizeParams, error) {
	if responseType != "code" {
		return authorizeParams{}, fmt.Errorf("unsupported response_type %q", responseType)
	}
	if method != "S256" {
		return authorizeParams{}, fmt.Errorf("unsupported code_challenge_method %q — only S256 is accepted", method)
	}
	if clientID == "" || redirectURI == "" || challenge == "" {
		return authorizeParams{}, fmt.Errorf("client_id, redirect_uri and code_challenge are required")
	}
	return authorizeParams{ClientID: clientID, RedirectURI: redirectURI, State: state, CodeChallenge: challenge, CodeChallengeMethod: method}, nil
}

// resolveAuthorizeClient looks up the named client and checks redirect_uri
// against its registered list — the one pair of checks both the GET consent
// screen and the POST decision must make before trusting anything else in the
// request, so neither duplicates it.
func (h *authHandlers) resolveAuthorizeClient(ctx context.Context, params authorizeParams) (db.OauthClient, error) {
	client, err := h.queries.GetOAuthClient(ctx, params.ClientID)
	if err != nil {
		return db.OauthClient{}, fiber.NewError(fiber.StatusBadRequest, "unknown client_id")
	}
	if !oauth2server.RedirectURIAllowed(client.RedirectUris, params.RedirectURI) {
		return db.OauthClient{}, fiber.NewError(fiber.StatusBadRequest, "redirect_uri is not registered for this client")
	}
	return client, nil
}

// redirectToClient sends the browser back to the client's redirect_uri with the
// given result params in the query string (not a fragment — a native app's
// loopback listener reads a 302's query, it cannot run JavaScript against one).
func redirectToClient(c *fiber.Ctx, redirectURI string, vals url.Values) error {
	loc, err := url.Parse(redirectURI)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid redirect_uri")
	}
	q := loc.Query()
	for k, v := range vals {
		q[k] = v
	}
	loc.RawQuery = q.Encode()
	return c.Redirect(loc.String(), fiber.StatusFound)
}

// resolveAuthorizeResource validates the optional RFC 8707 `resource` once the
// client and redirect_uri are trusted. An unknown resource is answered the way
// RFC 6749 4.1.2.1 says an authorize error is, by redirecting invalid_target to
// the client — which is why this runs after resolveAuthorizeClient: before it,
// the redirect_uri is not safe to send anything to.
func (h *authHandlers) resolveAuthorizeResource(c *fiber.Ctx, params *authorizeParams, values [][]byte) (done bool, err error) {
	resource, err := oauth2server.CanonicalResource(h.frontendOrigin, bytesToStrings(values))
	if err != nil {
		return true, redirectToClient(c, params.RedirectURI, url.Values{
			"error":             {"invalid_target"},
			"error_description": {"resource is not a freehire MCP endpoint"},
			"state":             {params.State},
		})
	}
	params.Resource = resource
	return false, nil
}

// isURLEncodedForm reports whether the body is application/x-www-form-urlencoded.
// The OAuth endpoints read `resource` from the urlencoded args, but every other
// field through FormValue, which also accepts multipart: a multipart body would
// therefore bind nothing and still succeed. RFC 6749 4.1.3 requires urlencoded
// at /token, and the consent form posts it, so anything else is refused.
func isURLEncodedForm(c *fiber.Ctx) bool {
	mediaType, _, _ := strings.Cut(c.Get(fiber.HeaderContentType), ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), fiber.MIMEApplicationForm)
}

func bytesToStrings(values [][]byte) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// OAuthAuthorize shows the consent screen, or sends a sessionless visitor to sign
// in first — the same two-outcome shape as ExtensionConnect. GET only ever reads;
// it issues nothing.
func (h *authHandlers) OAuthAuthorize(c *fiber.Ctx) error {
	params, err := parseAuthorizeParams(
		c.Query("response_type"), c.Query("client_id"), c.Query("redirect_uri"),
		c.Query("state"), c.Query("code_challenge"), c.Query("code_challenge_method"),
	)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	client, err := h.resolveAuthorizeClient(c.Context(), params)
	if err != nil {
		return err
	}

	if done, err := h.resolveAuthorizeResource(c, &params, c.Context().QueryArgs().PeekMulti("resource")); done {
		return err
	}

	userID, err := requireUserID(c)
	if err != nil {
		return c.Redirect(h.oauthSignInURL(params), fiber.StatusFound)
	}
	user, err := h.queries.GetUserByID(c.Context(), userID)
	if err != nil {
		return fmt.Errorf("read the account behind the oauth consent of user %d: %w", userID, err)
	}

	page, err := oauthConsentPage(client.ClientName, user.Email, h.frontendOrigin, params)
	if err != nil {
		return fmt.Errorf("render the oauth consent for user %d: %w", userID, err)
	}
	c.Type("html")
	return c.SendString(page)
}

// OAuthAuthorizeSubmit acts on the consent decision. On approval it mints a
// single-use authorization code and 302s it back to the client's redirect_uri in
// the query string (not a fragment — a native app's loopback listener reads a
// 302's query, it cannot run JavaScript against a fragment). Mounted on
// optionalCookie, not RequireAuth — same reasoning as ExtensionConnectSubmit:
// a session can lapse between the consent screen and this decision, and a bare
// 401 there would render into the client's browser instead of restarting the
// sign-in round trip. requireUserID below does the real gating: no session
// means no approval, same as RequireAuth would enforce, just with a redirect
// instead of a 401. A leaked API key still cannot approve a grant — a key
// alone never satisfies requireUserID's cookie check.
func (h *authHandlers) OAuthAuthorizeSubmit(c *fiber.Ctx) error {
	if !isURLEncodedForm(c) {
		return fiber.NewError(fiber.StatusBadRequest, "the consent decision must be application/x-www-form-urlencoded")
	}
	params, err := parseAuthorizeParams(
		"code", c.FormValue("client_id"), c.FormValue("redirect_uri"),
		c.FormValue("state"), c.FormValue("code_challenge"), c.FormValue("code_challenge_method"),
	)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// Re-validated here too: never trust that the GET step ran, same reasoning
	// as ExtensionConnectSubmit re-checking its own redirect allowlist.
	if _, err := h.resolveAuthorizeClient(c.Context(), params); err != nil {
		return err
	}
	if done, err := h.resolveAuthorizeResource(c, &params, c.Context().PostArgs().PeekMulti("resource")); done {
		return err
	}

	if c.FormValue("decision") == decisionSwitch {
		return h.switchAccount(c, h.oauthSignInURL(params))
	}
	userID, err := requireUserID(c)
	if err != nil {
		return c.Redirect(h.oauthSignInURL(params), fiber.StatusFound)
	}
	// Bind the code to the generation the approving SESSION carries — the tv
	// claim the auth middleware already validated against the account on the way
	// in — never a fresh read of users.token_version. A revocation that lands
	// between that validation and this insert leaves the code bound to the dead
	// generation, so it dies with the sessions that bump revokes instead of
	// capturing the new one. Read via SessionVersion rather than re-parsing:
	// Parse is what issued the id this handler acts on, and the two must agree.
	version, err := auth.SessionVersion(c)
	if err != nil {
		return fmt.Errorf("read session generation for user %d: %w", userID, err)
	}

	redirect := func(vals url.Values) error { return redirectToClient(c, params.RedirectURI, vals) }

	if c.FormValue("decision") != "allow" {
		return redirect(url.Values{"error": {"access_denied"}, "state": {params.State}})
	}

	code, hash, err := oauth2server.GenerateAuthorizationCode()
	if err != nil {
		return fmt.Errorf("generate authorization code for user %d: %w", userID, err)
	}
	expiresAt := time.Now().Add(authorizationCodeTTL)
	err = h.queries.CreateOAuthAuthorizationCode(c.Context(), db.CreateOAuthAuthorizationCodeParams{
		CodeHash:           hash,
		ClientID:           params.ClientID,
		UserID:             userID,
		RedirectUri:        params.RedirectURI,
		CodeChallenge:      params.CodeChallenge,
		IssuedTokenVersion: pgtype.Int4{Int32: version, Valid: true},
		ExpiresAt:          pgconv.Timestamptz(&expiresAt),
		Resource:           pgconv.Text(params.Resource),
	})
	if err != nil {
		return fmt.Errorf("persist authorization code for user %d: %w", userID, err)
	}

	return redirect(url.Values{"code": {code}, "state": {params.State}})
}

// oauthSignInURL sends a sessionless visitor to sign in, then back to this exact
// authorize request. Mirrors signInURL in extension_connect.go.
func (h *authHandlers) oauthSignInURL(params authorizeParams) string {
	back := url.Values{
		"response_type":         {"code"},
		"client_id":             {params.ClientID},
		"redirect_uri":          {params.RedirectURI},
		"state":                 {params.State},
		"code_challenge":        {params.CodeChallenge},
		"code_challenge_method": {params.CodeChallengeMethod},
	}
	if params.Resource != "" {
		back.Set("resource", resourceURI(h.frontendOrigin, params.Resource))
	}
	returnTo := "/api/v1/oauth/authorize?" + back.Encode()
	q := url.Values{"returnTo": {returnTo}, "via": {"web"}}
	return strings.TrimSuffix(h.frontendOrigin, "/") + "/signin?" + q.Encode()
}

// redirectDestination names, for a person, where approving sends the code. It is
// the one thing on the consent screen a registrant cannot choose freely —
// client_name is any string, so a client calling itself "Claude" is stopped only
// by the screen also saying "attacker.example.com". The URI was validated at
// registration and matched at authorize, so it parses.
func redirectDestination(redirectURI string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	switch host := u.Hostname(); {
	case strings.EqualFold(u.Scheme, "http"):
		return "an app on this computer (" + host + ")"
	case strings.EqualFold(u.Scheme, "https"):
		return host
	default:
		return "the " + u.Scheme + ":// app on this computer"
	}
}

// oauthConsentPage is the approval screen for every OAuth client — an AI app
// (ChatGPT, Claude) or a CLI on a loopback port. It names the redirect host
// beside the self-chosen client name, because the host is what a phishing
// registration cannot fake.
func oauthConsentPage(clientName, email, origin string, params authorizeParams) (string, error) {
	hidden := []hiddenField{
		{"client_id", params.ClientID},
		{"redirect_uri", params.RedirectURI},
		{"state", params.State},
		{"code_challenge", params.CodeChallenge},
		{"code_challenge_method", params.CodeChallengeMethod},
	}
	if params.Resource != "" {
		hidden = append(hidden, hiddenField{"resource", resourceURI(origin, params.Resource)})
	}
	return renderConsentScreen(consentScreen{
		Title:       clientName + " wants to access your freehire account",
		SignedIn:    email,
		Allows:      bearerAllows,
		Denies:      bearerDenies,
		Destination: redirectDestination(params.RedirectURI),
		Caution:     clientName + " chose its own name; freehire has not verified it. Allow only if you started this connection yourself.",
		Action:      "/api/v1/oauth/authorize",
		Hidden:      hidden,
		Origin:      origin,
	})
}

// resourceURI turns the bound resource PATH back into the absolute URI authorize accepts. The
// sign-in detour and the consent form both send it back to authorize; a bare path there was
// refused as invalid_target, failing every connector sign-in.
func resourceURI(origin, path string) string {
	return strings.TrimSuffix(origin, "/") + path
}
