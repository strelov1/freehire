// Package linkedinauth owns the credential the daily digest posts to LinkedIn with: how it is
// minted, where it is kept, and when it must be renewed or re-granted by a person.
//
// It is its own package because the credential EXPIRES, unlike a Discord webhook URL: a
// LinkedIn access token lasts 60 days, is renewed by a worker when LinkedIn permits it, and
// otherwise must be re-granted through a browser. socialdigest asks only for a token string.
//
// The renewal path is conditional: LinkedIn issues programmatic refresh tokens to approved
// Marketing Developer Platform partners only, so whether we get one is known only from the
// token response. With a refresh token the worker renews silently; without one it warns
// while a person can still act.
package linkedinauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Channel is the name this credential is stored and logged under. It matches the
// publisher's channel name in socialdigest and is read back from storage, so neither may
// be renamed without a migration.
const Channel = "linkedin"

// Scope is what the digest needs and nothing more: permission to post for the
// organization whose page we own. Changing it invalidates every existing token (LinkedIn
// discards previous grants), so a person must sign in again.
const Scope = "w_organization_social"

// LinkedIn's OAuth lives on www.linkedin.com, its APIs on api.linkedin.com; an OAuth
// request to the API host answers 404.
const authorizeURL = "https://www.linkedin.com/oauth/v2/authorization"

// tokenEndpoint is where both grants are exchanged. A variable so tests can point it at
// a stub.
var tokenEndpoint = "https://www.linkedin.com/oauth/v2/accessToken"

// Credentials are the application's own identity, from configuration.
//
// RedirectURI must be registered on the LinkedIn application, absolute and HTTPS, and
// sent IDENTICALLY in both legs of the flow; LinkedIn answers a mismatch with a 400.
type Credentials struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Configured reports whether a sign-in can be attempted at all. A half-configured
// deployment must look "off", not like a flow that fails visibly.
func (c Credentials) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURI != ""
}

// Token is a minted credential with absolute times rather than the "seconds remaining"
// LinkedIn returns, since everything downstream compares against a clock.
type Token struct {
	AccessToken string
	ExpiresAt   time.Time

	// RefreshToken is empty when LinkedIn did not issue one, the expected case outside the
	// Marketing Developer Platform. Its emptiness decides whether renewal is possible.
	RefreshToken     string
	RefreshExpiresAt time.Time

	Scope string
}

// Renewable reports whether this token can be exchanged for a fresh one without a person.
func (t Token) Renewable() bool { return t.RefreshToken != "" }

// AuthorizeURL is the address a person opens to grant the application access to the
// company page. cmd/linkedin-auth prints it rather than the API serving it: one operator
// runs this every sixty days.
//
// state is returned on the redirect but deliberately not verified: it defends a browser
// session against a substituted code, and this flow has none (one person copies the code
// from their own address bar into a command). It is sent so the redirect is identifiable
// in a log.
func (c Credentials) AuthorizeURL(state string) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {c.ClientID},
		"redirect_uri":  {c.RedirectURI},
		"scope":         {Scope},
	}
	if state != "" {
		q.Set("state", state)
	}
	return authorizeURL + "?" + q.Encode()
}

// Exchange turns an authorization code into a token. The code is single-use and lives 30
// minutes, so a failure is ordinarily "you took too long".
func (c Credentials) Exchange(ctx context.Context, httpc *http.Client, code string) (Token, error) {
	return c.postToken(ctx, httpc, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"redirect_uri":  {c.RedirectURI},
	})
}

// Refresh exchanges a refresh token for a fresh access token.
//
// The refresh token's own expiry does NOT move (365 days from sign-in), so a person still
// owes one sign-in a year.
func (c Credentials) Refresh(ctx context.Context, httpc *http.Client, refreshToken string) (Token, error) {
	return c.postToken(ctx, httpc, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	})
}

// tokenResponse is LinkedIn's answer to both grants. refresh_token and its expiry are
// absent unless the application may use programmatic refresh tokens.
type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`

	// The OAuth error shape, returned with a 400; "authorization code expired" and
	// "redirect uri does not match" look identical from the status alone.
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// now is the clock expiry is anchored against; a variable so tests can pin it.
var now = time.Now

func (c Credentials) postToken(ctx context.Context, httpc *http.Client, form url.Values) (Token, error) {
	if httpc == nil {
		httpc = defaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpc.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("linkedin token endpoint: %w", err)
	}
	defer resp.Body.Close()

	// Bounded: this reaches logs and a terminal, and some failures are HTML pages.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return Token{}, fmt.Errorf("linkedin token endpoint: read body: %w", err)
	}

	var parsed tokenResponse
	if jsonErr := json.Unmarshal(body, &parsed); jsonErr != nil {
		return Token{}, fmt.Errorf("linkedin token endpoint: status %d: unreadable body: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if parsed.Error != "" {
		return Token{}, fmt.Errorf("linkedin token endpoint: status %d: %s: %s",
			resp.StatusCode, parsed.Error, parsed.ErrorDescription)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Token{}, fmt.Errorf("linkedin token endpoint: status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// A 200 without a token would store an empty credential that fails later, at publish
	// time, as an untraceable 401.
	if parsed.AccessToken == "" {
		return Token{}, fmt.Errorf("linkedin token endpoint: status %d: no access_token in response", resp.StatusCode)
	}

	issued := now().UTC()
	tok := Token{
		AccessToken:  parsed.AccessToken,
		ExpiresAt:    issued.Add(time.Duration(parsed.ExpiresIn) * time.Second),
		RefreshToken: parsed.RefreshToken,
		Scope:        parsed.Scope,
	}
	if parsed.RefreshTokenExpiresIn > 0 {
		tok.RefreshExpiresAt = issued.Add(time.Duration(parsed.RefreshTokenExpiresIn) * time.Second)
	}
	return tok, nil
}

// defaultClient is used when a caller passes none. A plain http.Client, not safehttp: the
// host is a fixed vendor endpoint, not user input, so there is no SSRF surface.
var defaultClient = &http.Client{Timeout: 20 * time.Second}
