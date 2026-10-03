package handler

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// oauthTokenApp mounts the token route on a handler with no DB. It is public
// (the caller authenticates with the code and PKCE verifier, not a session), so
// these cases only exercise validation that runs before any query.
func oauthTokenApp() *fiber.App {
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/token", h.OAuthToken)
	return app
}

func TestOAuthToken_RejectsUnsupportedGrantType(t *testing.T) {
	app := oauthTokenApp()
	form := url.Values{"grant_type": {"client_credentials"}}
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

func TestOAuthToken_RejectsMissingFields(t *testing.T) {
	app := oauthTokenApp()
	form := url.Values{"grant_type": {"authorization_code"}}
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
