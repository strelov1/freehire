package gmailsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/strelov1/freehire/internal/safehttp"
)

// gmailReadonlyScope is the restricted read scope; its consent screen requires
// Google verification for public use (testing-mode test users until then).
const gmailReadonlyScope = "https://www.googleapis.com/auth/gmail.readonly"

const gmailProfileURL = "https://gmail.googleapis.com/gmail/v1/users/me/profile"

// gmailHTTPTimeout bounds the OAuth exchange and Gmail API round-trips.
const gmailHTTPTimeout = 15 * time.Second

// Connector runs the "Connect Gmail" incremental OAuth flow on the existing
// Google client: it builds the consent URL and exchanges the callback code for a
// refresh token + the connected address. Distinct from the sign-in Provider
// (which stores no tokens) — here we need offline access and a stored token.
type Connector struct {
	cfg *oauth2.Config
}

// NewConnector builds the Gmail connector from the Google OAuth credentials. The
// callback is origin + /api/v1/me/gmail/callback (register it on the client).
func NewConnector(clientID, clientSecret, origin string) *Connector {
	return &Connector{cfg: &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     endpoints.Google,
		RedirectURL:  origin + "/api/v1/me/gmail/callback",
		Scopes:       []string{gmailReadonlyScope},
	}}
}

// AuthCodeURL builds the consent URL. offline + forced consent yield a refresh
// token; include_granted_scopes keeps the user's existing sign-in grant
// (incremental authorization).
func (c *Connector) AuthCodeURL(state string) string {
	return c.cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	)
}

// Exchange turns the callback code into a refresh token and the connected Gmail
// address. It errors if Google returned no refresh token (consent not offline).
func (c *Connector) Exchange(ctx context.Context, code string) (refreshToken, email string, err error) {
	ctx = guardedContext(ctx)
	tok, err := c.cfg.Exchange(ctx, code)
	if err != nil {
		return "", "", fmt.Errorf("gmail: exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", "", errors.New("gmail: no refresh token (consent was not offline)")
	}
	email, err = c.fetchEmail(ctx, c.cfg.Client(ctx, tok))
	if err != nil {
		return "", "", err
	}
	return tok.RefreshToken, email, nil
}

// TokenSource mints access tokens from a stored refresh token (used by the sync
// worker), refreshing transparently.
func (c *Connector) TokenSource(ctx context.Context, refreshToken string) oauth2.TokenSource {
	return c.cfg.TokenSource(guardedContext(ctx), &oauth2.Token{RefreshToken: refreshToken})
}

// HTTPClient returns a token-bearing HTTP client for the Gmail API, minting and
// refreshing access tokens from the stored refresh token. Used to build a live
// GmailReader per user in the sync worker.
func (c *Connector) HTTPClient(ctx context.Context, refreshToken string) *http.Client {
	gctx := guardedContext(ctx)
	return oauth2.NewClient(gctx, c.cfg.TokenSource(gctx, &oauth2.Token{RefreshToken: refreshToken}))
}

// ReaderFactory builds the per-user GmailReader factory the sync Worker needs,
// minting a token-bearing Gmail client from each user's refresh token — the
// single source shared by the cron worker and the on-demand sync handler.
func (c *Connector) ReaderFactory() ReaderFactory {
	return func(ctx context.Context, refreshToken string) GmailReader {
		return NewAPIReader(c.HTTPClient(ctx, refreshToken))
	}
}

// Revoke best-effort revokes a refresh token with Google (disconnect). A failure
// is ignored — we purge our copy regardless.
func (c *Connector) Revoke(ctx context.Context, refreshToken string) {
	client := safehttp.NewClient(gmailHTTPTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/revoke?token="+url.QueryEscape(refreshToken), nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}

// fetchEmail reads the mailbox address via the Gmail profile (needs only the
// gmail.readonly scope we already hold — no extra email scope).
func (c *Connector) fetchEmail(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gmailProfileURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gmail: profile: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gmail: profile: %s", resp.Status)
	}
	var body struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("gmail: profile decode: %w", err)
	}
	return body.EmailAddress, nil
}

// guardedContext routes the OAuth exchange and API calls through the SSRF-guarded
// client, matching every other outbound fetch in this service (defense in depth).
func guardedContext(ctx context.Context) context.Context {
	if _, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, safehttp.NewClient(gmailHTTPTimeout))
}
