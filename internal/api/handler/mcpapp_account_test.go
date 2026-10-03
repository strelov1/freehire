package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func mcpAccountApp(assistants *assistantHandlers) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// A nil throttler fails open (ratelimit.Middleware's documented behavior),
	// so the zero-value middleware needs no fake here.
	mw := middleware{}
	api := app.Group("/api/v1")
	mountMCPAccount(api, mw, assistants)
	return app
}

func TestMCPAccountMount_RejectsMissingBearer(t *testing.T) {
	app := mcpAccountApp(&assistantHandlers{})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/mcp/account", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
