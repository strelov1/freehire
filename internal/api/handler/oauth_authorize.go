package handler

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

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

	if _, err := requireUserID(c); err != nil {
		return c.Redirect(h.oauthSignInURL(params), fiber.StatusFound)
	}

	c.Type("html")
	return c.SendString(oauthConsentPage(client.ClientName, params))
}

// OAuthAuthorizeSubmit acts on the consent decision. On approval it mints a
// single-use authorization code and 302s it back to the client's redirect_uri in
// the query string (not a fragment — a native app's loopback listener reads a
// 302's query, it cannot run JavaScript against a fragment). Cookie-only
// (RequireAuth): a leaked API key must not be able to approve a new MCP grant.
func (h *authHandlers) OAuthAuthorizeSubmit(c *fiber.Ctx) error {
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

	userID, err := requireUserID(c)
	if err != nil {
		return c.Redirect(h.oauthSignInURL(params), fiber.StatusFound)
	}

	redirect := func(vals url.Values) error {
		loc, err := url.Parse(params.RedirectURI)
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

	if c.FormValue("decision") != "allow" {
		return redirect(url.Values{"error": {"access_denied"}, "state": {params.State}})
	}

	code, hash, err := oauth2server.GenerateAuthorizationCode()
	if err != nil {
		return fmt.Errorf("generate authorization code for user %d: %w", userID, err)
	}
	expiresAt := time.Now().Add(authorizationCodeTTL)
	err = h.queries.CreateOAuthAuthorizationCode(c.Context(), db.CreateOAuthAuthorizationCodeParams{
		CodeHash:      hash,
		ClientID:      params.ClientID,
		UserID:        userID,
		RedirectUri:   params.RedirectURI,
		CodeChallenge: params.CodeChallenge,
		ExpiresAt:     pgconv.Timestamptz(&expiresAt),
	})
	if err != nil {
		return fmt.Errorf("persist authorization code for user %d: %w", userID, err)
	}

	return redirect(url.Values{"code": {code}, "state": {params.State}})
}

// oauthSignInURL sends a sessionless visitor to sign in, then back to this exact
// authorize request. Mirrors signInURL in extension_connect.go.
func (h *authHandlers) oauthSignInURL(params authorizeParams) string {
	returnTo := "/api/v1/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {params.ClientID},
		"redirect_uri":          {params.RedirectURI},
		"state":                 {params.State},
		"code_challenge":        {params.CodeChallenge},
		"code_challenge_method": {params.CodeChallengeMethod},
	}.Encode()
	q := url.Values{"returnTo": {returnTo}, "via": {"web"}}
	return strings.TrimSuffix(h.frontendOrigin, "/") + "/signin?" + q.Encode()
}

// oauthConsentPage is a minimal, dependency-free HTML form — same reasoning as
// extension_connect.go's consentPage: this is a one-screen, server-rendered
// surface, not a reason to round-trip through the SPA build.
func oauthConsentPage(clientName string, params authorizeParams) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Connect %s</title></head>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">
  <h1>Connect %s to your freehire account</h1>
  <p>%s will be able to search jobs and manage your tracked applications on your behalf.</p>
  <form method="post" action="/api/v1/oauth/authorize">
    <input type="hidden" name="client_id" value="%s">
    <input type="hidden" name="redirect_uri" value="%s">
    <input type="hidden" name="state" value="%s">
    <input type="hidden" name="code_challenge" value="%s">
    <input type="hidden" name="code_challenge_method" value="%s">
    <button type="submit" name="decision" value="allow">Allow</button>
    <button type="submit" name="decision" value="deny">Deny</button>
  </form>
</body>
</html>`,
		html.EscapeString(clientName), html.EscapeString(clientName), html.EscapeString(clientName),
		html.EscapeString(params.ClientID), html.EscapeString(params.RedirectURI), html.EscapeString(params.State),
		html.EscapeString(params.CodeChallenge), html.EscapeString(params.CodeChallengeMethod))
}
