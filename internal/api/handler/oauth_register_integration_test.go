//go:build integration

// Integration test for the dynamic-registration HTTP flow against a real
// Postgres: a valid request persists a client row and returns a usable
// client_id. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRegisterOAuthClient_PersistsAndReturnsClientID(t *testing.T) {
	pool := startPostgres(t)
	h := &authHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/register", h.RegisterOAuthClient)

	body, _ := json.Marshal(map[string]any{
		"client_name":   "Test MCP Client",
		"redirect_uris": []string{"http://127.0.0.1:51739/callback"},
	})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusCreated)
	}
	var out registerClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ClientID == "" {
		t.Error("client_id is empty")
	}
	if out.TokenEndpointAuthMethod != "none" {
		t.Errorf("token_endpoint_auth_method = %q, want \"none\"", out.TokenEndpointAuthMethod)
	}

	got, err := h.queries.GetOAuthClient(req.Context(), out.ClientID)
	if err != nil {
		t.Fatalf("GetOAuthClient: %v", err)
	}
	if got.ClientName != "Test MCP Client" {
		t.Errorf("stored client_name = %q, want %q", got.ClientName, "Test MCP Client")
	}
}

// Gemini registers more than five callbacks in one request; the bound used to be
// five and refused it with a 400 every time.
func TestRegisterOAuthClient_AcceptsManyRedirectURIs(t *testing.T) {
	pool := startPostgres(t)
	h := &authHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/oauth/register", h.RegisterOAuthClient)

	uris := manyRedirectURIs(maxRedirectURIs)
	body, _ := json.Marshal(map[string]any{"client_name": "Gemini", "redirect_uris": uris})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusCreated)
	}
	var out registerClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.RedirectURIs) != len(uris) {
		t.Errorf("redirect_uris = %d, want %d", len(out.RedirectURIs), len(uris))
	}
}
