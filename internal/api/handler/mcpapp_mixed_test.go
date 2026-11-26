package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/mcpapp"
)

// A caller-supplied identity header never reaches the MCP handler: with no bearer the
// request is refused before it, whatever header or cookie it carries.
func TestMCPAuthenticateIgnoresAForgedIdentity(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/", mcpAuthenticate(&assistantHandlers{}, testMCPOrigin), func(c *fiber.Ctx) error {
		t.Error("a request with no bearer reached the handler")
		return c.SendStatus(204)
	})
	for _, authorization := range []string{"", "Basic forged"} {
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/", nil)
		req.Header.Set(mcpapp.AccountUserIDHeader, "11532")
		req.Header.Set("Cookie", "session=forged")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("auth %q status %d want 401", authorization, resp.StatusCode)
		}
	}
}

// Someone who opens the connector URL in a browser reads where to add it, still as a 401.
func TestMCPAuthenticateMissingBearerPointsToConnectPage(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/", mcpAuthenticate(&assistantHandlers{}, testMCPOrigin), func(c *fiber.Ctx) error {
		t.Error("a request with no bearer reached the handler")
		return c.SendStatus(204)
	})
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusUnauthorized || !strings.Contains(string(body), testMCPOrigin+"/mcp") {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}
