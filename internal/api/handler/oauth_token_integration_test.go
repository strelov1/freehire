//go:build integration

// Integration tests for the OAuth token HTTP flow against a real Postgres: a
// valid code + verifier exchange for an access token, a wrong verifier is
// rejected, and a code cannot be redeemed twice. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func challengeForVerifier(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// approveTestAuthorization drives the real authorize-submit handler (decision=allow)
// and returns the authorization code it redirects with — the same path a real
// client's consent click takes, rather than inserting a code row directly.
func approveTestAuthorization(t *testing.T, h *authHandlers, iss *auth.Issuer, userID int64, clientID, redirectURI, challenge string) string {
	t.Helper()
	app := oauthAuthorizeApp(h, iss)
	cookie, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	form := url.Values{
		"decision": {"allow"}, "client_id": {clientID}, "redirect_uri": {redirectURI},
		"state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("approve: app.Test: %v", err)
	}
	defer resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("approve: parse Location: %v", err)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("approve: redirect carried no code")
	}
	return code
}

func TestOAuthToken_ExchangesValidCode(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-1@example.test")
	seedOAuthTestClient(t, queries, "client-tok-1", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss}

	verifier := "a-high-entropy-verifier-the-client-generated-1234567890"
	code := approveTestAuthorization(t, h, iss, userID, "client-tok-1", "http://127.0.0.1:9999/callback", challengeForVerifier(verifier))

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/token", h.OAuthToken)

	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"client-tok-1"},
		"redirect_uri": {"http://127.0.0.1:9999/callback"}, "code_verifier": {verifier},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AccessToken == "" {
		t.Error("access_token is empty")
	}
	if out.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", out.TokenType)
	}
	if out.ExpiresIn != int(accessTokenTTL.Seconds()) {
		t.Errorf("expires_in = %d, want %d", out.ExpiresIn, int(accessTokenTTL.Seconds()))
	}
}

func TestOAuthToken_RejectsWrongVerifier(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-2@example.test")
	seedOAuthTestClient(t, queries, "client-tok-2", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss}

	code := approveTestAuthorization(t, h, iss, userID, "client-tok-2", "http://127.0.0.1:9999/callback", challengeForVerifier("the-real-verifier"))

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/token", h.OAuthToken)

	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"client-tok-2"},
		"redirect_uri": {"http://127.0.0.1:9999/callback"}, "code_verifier": {"not-the-real-verifier"},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestOAuthToken_RejectsReplayedCode(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "token-3@example.test")
	seedOAuthTestClient(t, queries, "client-tok-3", []string{"http://127.0.0.1:9999/callback"})
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss}

	verifier := "a-high-entropy-verifier-the-client-generated-1234567890"
	code := approveTestAuthorization(t, h, iss, userID, "client-tok-3", "http://127.0.0.1:9999/callback", challengeForVerifier(verifier))

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/token", h.OAuthToken)
	body := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"client-tok-3"},
		"redirect_uri": {"http://127.0.0.1:9999/callback"}, "code_verifier": {verifier},
	}.Encode()

	first := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(body))
	first.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	firstResp, err := app.Test(first)
	if err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != fiber.StatusOK {
		t.Fatalf("first exchange status = %d, want 200", firstResp.StatusCode)
	}

	second := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(body))
	second.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondResp, err := app.Test(second)
	if err != nil {
		t.Fatalf("second exchange: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("replayed code: status = %d, want 400", secondResp.StatusCode)
	}
}
