package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// oauthRegisterApp mounts the registration route on a handler with no DB. It is
// public (RFC 7591 — a client self-registers with no user context yet), so no
// auth middleware sits in front of it; these cases only exercise validation that
// runs before any query, mirroring how api_keys_test.go splits DB-free cases from
// the integration tests that need a real Postgres.
func oauthRegisterApp() *fiber.App {
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/register", h.RegisterOAuthClient)
	return app
}

func TestRegisterOAuthClient_RejectsMissingRedirectURI(t *testing.T) {
	app := oauthRegisterApp()
	body, _ := json.Marshal(map[string]any{"client_name": "No Redirect"})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestRegisterOAuthClient_RejectsMissingClientName(t *testing.T) {
	app := oauthRegisterApp()
	body, _ := json.Marshal(map[string]any{"redirect_uris": []string{"http://127.0.0.1:9999/callback"}})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}
