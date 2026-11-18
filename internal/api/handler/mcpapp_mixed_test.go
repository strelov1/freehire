package handler

import (
	"context"
	"net/http/httptest"
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
