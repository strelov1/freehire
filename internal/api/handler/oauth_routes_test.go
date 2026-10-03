package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestMCPOAuthRoutes_LiveAtAPIV1NotUnderAuth is a regression test for a real
// deploy incident: every MCP OAuth handler was written and unit-tested against
// bare fiber apps mounting it directly at /api/v1/oauth/..., but the real
// register() call nested it under authGroup (/api/v1/auth/...) instead — a
// mismatch no existing test caught, because none of them drove requests through
// the actual router tree authHandlers.register builds. Discovered only in
// production: the metadata document and the consent page both advertised
// /api/v1/oauth/* while the live binary only answered on
// /api/v1/auth/oauth/*, 404ing every real client.
//
// This drives the real register() tree, the same way
// TestPublicReadLimiters_KeyWhatTheMountedChainCanSee does for the public-read
// guard, so a future path move is caught here instead of on the live site.
func TestMCPOAuthRoutes_LiveAtAPIV1NotUnderAuth(t *testing.T) {
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	passthrough := func(c *fiber.Ctx) error { return c.Next() }
	h.register(app.Group("/api/v1"), middleware{cookie: passthrough, optionalCookie: passthrough})

	for _, tc := range []struct{ method, path string }{
		{fiber.MethodPost, "/api/v1/oauth/register"},
		{fiber.MethodGet, "/api/v1/oauth/metadata/authorization-server"},
		{fiber.MethodGet, "/api/v1/oauth/metadata/protected-resource"},
		{fiber.MethodGet, "/api/v1/oauth/authorize"},
		{fiber.MethodPost, "/api/v1/oauth/authorize"},
		{fiber.MethodPost, "/api/v1/oauth/token"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode == fiber.StatusNotFound {
				t.Errorf("%s %s = 404; MCP OAuth routes must be mounted on api directly, not authGroup", tc.method, tc.path)
			}
		})
	}

	t.Run("not nested under /auth", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/auth/oauth/register", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("GET /api/v1/auth/oauth/register = %d, want 404 (it must not live under /auth)", resp.StatusCode)
		}
	})
}
