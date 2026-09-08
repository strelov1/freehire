// Package linkedinauth owns the credential the daily digest posts to LinkedIn with: how it is
// minted, where it is kept, and when it must be renewed or re-granted by a person.
//
// It exists as its own package because that credential EXPIRES, which is the one thing every
// other channel of this feature does not do. A Discord webhook URL is static configuration; a
// LinkedIn access token lasts 60 days, is renewed by a worker when LinkedIn permits it, and
// otherwise has to be re-granted through a browser before it dies. None of that is knowledge
// the digest's editorial rules should carry, so socialdigest asks only for a token string.
//
// WHY THE RENEWAL PATH IS CONDITIONAL. LinkedIn issues programmatic refresh tokens to approved
// Marketing Developer Platform partners only, and the Community Management API — the product
// that lets us post to our own company page — is not that program. Whether our application
// gets one is therefore not knowable from the documentation, only from the token response.
// So both outcomes are first-class here: with a refresh token the worker renews silently, and
// without one it warns while there is still time for a person to act. Shipping only the happy
// path would mean discovering the answer as a digest that stopped publishing.
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

// Channel is the name this credential is stored and logged under. It matches the publisher's
// channel name in socialdigest, and both are read back from storage, so neither may be renamed
// without a migration.
const Channel = "linkedin"

// Scope is what the digest needs and nothing more: permission to post on behalf of the
// organization whose page we own. LinkedIn requires the member to consent to every requested
// scope at once, so asking for a read scope we do not use would make the consent screen
// broader than the feature.
//
// Note that changing this string invalidates every existing token — LinkedIn discards previous
// grants when the requested scope changes — so a change here is a change that requires a
// person to sign in again.
const Scope = "w_organization_social"

// Endpoints. LinkedIn's OAuth lives on www.linkedin.com while its APIs live on
// api.linkedin.com; sending an OAuth request to the API host answers 404, which reads like a
// retired endpoint rather than a wrong host.
const authorizeURL = "https://www.linkedin.com/oauth/v2/authorization"

// tokenEndpoint is where both grants are exchanged. A variable rather than a constant so the
// tests can point it at a stub: what goes to it is form-encoded, and a wrong grant_type or a
// dropped redirect_uri is invisible until LinkedIn refuses it in production.
var tokenEndpoint = "https://www.linkedin.com/oauth/v2/accessToken"

// Credentials are the application's own identity, from configuration.
//
// RedirectURI must be one of the redirect URLs registered on the LinkedIn application, must be
// absolute and HTTPS, and must be sent IDENTICALLY in both legs of the flow — LinkedIn
// compares them and answers a mismatch with a 400 naming the redirect_uri rather than the
// thing that is actually wrong.
type Credentials struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Configured reports whether a sign-in can be attempted at all. Every field is load-bearing,
// so a deployment holding two of three has not configured this channel — it has half-configured
// it, which must look like "off" rather than like a flow that fails where somebody can see it.
func (c Credentials) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURI != ""
}

// Token is a minted credential as this package hands it around: absolute times rather than the
// "seconds remaining" LinkedIn returns, because everything downstream compares against a clock
// and a duration would have to be re-anchored at every one of those comparisons.
type Token struct {
	AccessToken string
	ExpiresAt   time.Time

	// RefreshToken is empty when LinkedIn did not issue one — the expected case outside the
	// Marketing Developer Platform. Its emptiness is what decides whether renewal is possible,
	// so it is a plain string rather than a pointer: there is no difference here between
	// "absent" and "empty" worth representing.
	RefreshToken     string
	RefreshExpiresAt time.Time

	Scope string
}

// Renewable reports whether this token can be exchanged for a fresh one without a person.
func (t Token) Renewable() bool { return t.RefreshToken != "" }

// AuthorizeURL is the address a person opens to grant the application access to the company
// page. It is printed by cmd/linkedin-auth rather than served by the API, because this is run
// once every sixty days by one operator — a route, a callback handler and a state cookie would
// be a login page for an audience of one.
//
// state is passed through to LinkedIn and returned on the redirect. It is not verified on the
// way back and this is deliberate: state defends a BROWSER SESSION against a substituted
// authorization code, and this flow has no session — the same person opens the URL, reads the
// code out of their own address bar and hands it to a command on a host only they can reach.
// A value is still sent so the redirect is distinguishable in a log.
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
// minutes, so a failure here is ordinarily "you took too long", which the error says.
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
// The refresh token's own expiry does NOT move: it keeps the life it was granted at sign-in
// (365 days), so a deployment that renews forever still owes a person one sign-in a year.
func (c Credentials) Refresh(ctx context.Context, httpc *http.Client, refreshToken string) (Token, error) {
	return c.postToken(ctx, httpc, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	})
}

// tokenResponse is LinkedIn's answer to both grants. refresh_token and its expiry are absent
// unless the application is authorized for programmatic refresh tokens, so they are read as
// zero values rather than required.
type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`

	// The OAuth error shape, which LinkedIn returns with a 400 and a body worth repeating —
	// "authorization code expired" and "redirect uri does not match" are the two failures this
	// flow actually produces, and they are indistinguishable from the status alone.
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// now is the clock these exchanges anchor expiry against. A package variable so the tests can
// pin it: every value this package produces is a deadline, and a deadline computed from an
// untestable clock is a deadline nobody has checked.
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

	// Bounded: this ends up in a log line and in an operator's terminal, and LinkedIn answers
	// some failures with an HTML error page rather than JSON.
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
	// A 200 carrying no token is not a token. It has been seen when a required parameter is
	// accepted but ignored, and treating it as success would store an empty credential that
	// fails much later, at publish time, as a 401 nobody can trace back to here.
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

// defaultClient is used when a caller passes none. A plain http.Client and not safehttp: the
// host is a fixed vendor endpoint compiled into this file, not user input, so there is no SSRF
// surface — the same reasoning as the Discord publisher beside it.
var defaultClient = &http.Client{Timeout: 20 * time.Second}
