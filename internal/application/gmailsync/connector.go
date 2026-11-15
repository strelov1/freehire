package gmailsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// GmailReadonlyScope is the restricted read scope; its consent screen requires Google
// verification for public use. Exported because the handler that records a grant's scopes
// must spell them as the worker's filter reads them.
const GmailReadonlyScope = "https://www.googleapis.com/auth/gmail.readonly"

// CalendarScope is the read-only calendar scope, asked for on the calendar consent beside
// CalendarEventsScope.
//
// Declared here, once: internal/application/calsync filters connections on the scopes recorded
// at consent, so a differently spelled copy would find nobody and report a clean run.
const CalendarScope = "https://www.googleapis.com/auth/calendar.readonly"

// CalendarEventsScope is the calendar WRITE scope, asked for on the same calendar consent
// as CalendarScope so a mentor's bookings get a minted Meet link. A grant recorded before the
// two were one consent may hold either alone, so readers still check each scope.
const CalendarEventsScope = "https://www.googleapis.com/auth/calendar.events"

const gmailProfileURL = "https://gmail.googleapis.com/gmail/v1/users/me/profile"

// gmailHTTPTimeout bounds the OAuth exchange and Gmail API round-trips.
const gmailHTTPTimeout = 15 * time.Second

// Connector runs the "Connect Gmail" incremental OAuth flow on the existing
// Google client: it builds the consent URL and exchanges the callback code for a
// refresh token + the connected address. Distinct from the sign-in Provider
// (which stores no tokens) — here we need offline access and a stored token.
type Connector struct {
	cfg *oauth2.Config
	// calendarRedirect is the callback for the calendar consent. Its own path, because
	// Google matches the redirect exactly and the mail and calendar flows land in
	// different handlers.
	calendarRedirect string
}

// NewConnector builds the Gmail connector from the Google OAuth credentials. The
// callback is origin + /api/v1/me/gmail/callback (register it on the client).
func NewConnector(clientID, clientSecret, origin string) *Connector {
	return &Connector{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoints.Google,
			RedirectURL:  origin + "/api/v1/me/gmail/callback",
			Scopes:       []string{GmailReadonlyScope},
		},
		calendarRedirect: origin + "/api/v1/me/calendar/callback",
	}
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

// calendarScopes is what the one calendar consent asks for: read, for interviews on the
// Tracking calendar and a mentor's busy time; write, for a mentor's minted Meet links.
var calendarScopes = []string{CalendarScope, CalendarEventsScope}

// calendarConfig is the client config with the calendar consent's scopes and redirect.
func (c *Connector) calendarConfig() *oauth2.Config {
	cfg := *c.cfg
	cfg.Scopes = calendarScopes
	cfg.RedirectURL = c.calendarRedirect
	return &cfg
}

// CalendarAuthCodeURL builds the consent URL for the calendar, on its own.
//
// Separate from AuthCodeURL: connecting a mailbox must never quietly ask for the calendar.
// Incremental, so the returned refresh token covers whatever was already granted.
func (c *Connector) CalendarAuthCodeURL(state string) string {
	return c.calendarConfig().AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	)
}

// Exchange turns the callback code into a refresh token and the connected Gmail
// address. It errors if Google returned no refresh token (consent not offline).
func (c *Connector) Exchange(ctx context.Context, code string) (refreshToken, email string, scopes []string, err error) {
	ctx = safehttp.GuardedOAuth2Context(ctx, gmailHTTPTimeout)
	tok, err := c.cfg.Exchange(ctx, code)
	if err != nil {
		return "", "", nil, fmt.Errorf("gmail: exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", "", nil, errors.New("gmail: no refresh token (consent was not offline)")
	}
	email, err = c.fetchEmail(ctx, c.cfg.Client(ctx, tok))
	if err != nil {
		return "", "", nil, err
	}
	return tok.RefreshToken, email, GrantedScopes(tok), nil
}

// GrantedScopes reads what Google says the grant actually covers.
//
// Authoritative, so recorded as-is rather than unioned with what we held: a union only grows,
// so a candidate who reconnects mail alone would keep a calendar scope they no longer have,
// and cal-sync would read the resulting 403 as revocation and flip the SHARED status.
//
// A response without the field is not evidence of no scopes; the caller keeps what it had.
func GrantedScopes(tok *oauth2.Token) []string {
	raw, _ := tok.Extra("scope").(string)
	return strings.Fields(raw)
}

// ExchangeCalendar turns the calendar callback's code into a refresh token.
//
// Separate from Exchange, which reads the mailbox address via the Gmail profile API and so
// needs the mail scope a calendar-only candidate lacks. The token covers everything granted
// in total (incremental consent).
func (c *Connector) ExchangeCalendar(ctx context.Context, code string) (refreshToken string, scopes []string, err error) {
	tok, err := c.calendarConfig().Exchange(safehttp.GuardedOAuth2Context(ctx, gmailHTTPTimeout), code)
	if err != nil {
		return "", nil, fmt.Errorf("calendar: exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", nil, errors.New("calendar: no refresh token (consent was not offline)")
	}
	return tok.RefreshToken, GrantedScopes(tok), nil
}

// TokenSource mints access tokens from a stored refresh token (used by the sync
// worker), refreshing transparently.
func (c *Connector) TokenSource(ctx context.Context, refreshToken string) oauth2.TokenSource {
	return c.cfg.TokenSource(safehttp.GuardedOAuth2Context(ctx, gmailHTTPTimeout), &oauth2.Token{RefreshToken: refreshToken})
}

// HTTPClient returns a token-bearing HTTP client for the Gmail API, minting and
// refreshing access tokens from the stored refresh token. Used to build a live
// GmailReader per user in the sync worker.
func (c *Connector) HTTPClient(ctx context.Context, refreshToken string) *http.Client {
	gctx := safehttp.GuardedOAuth2Context(ctx, gmailHTTPTimeout)
	return oauth2.NewClient(gctx, c.cfg.TokenSource(gctx, &oauth2.Token{RefreshToken: refreshToken}))
}

// ReaderFactory builds the per-user GmailReader factory the sync Worker needs,
// minting a token-bearing Gmail client from each user's refresh token — the
// single source shared by the cron worker and the on-demand sync handler. The
// learned domains are threaded in per run so the query reflects the current cache.
func (c *Connector) ReaderFactory() ReaderFactory {
	return func(ctx context.Context, refreshToken string, learned []string) GmailReader {
		return NewAPIReader(c.HTTPClient(ctx, refreshToken), learned)
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
