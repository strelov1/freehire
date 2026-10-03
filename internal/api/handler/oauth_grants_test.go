package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// oauthGrantsApp mounts the Connected-devices routes behind RequireAuth
// (cookie-only), the same gate they run behind in production. Cookie-only
// enforcement (a bearer key must not reach these routes) is covered by
// TestAPIKeysManagement_IsCookieOnly's pattern applied here.
func oauthGrantsApp() *fiber.App {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/oauth-grants", auth.RequireAuth(iss, testVersions), h.ListOAuthGrants)
	app.Delete("/api/v1/me/oauth-grants/:id", auth.RequireAuth(iss, testVersions), h.RevokeOAuthGrant)
	return app
}

func TestOAuthGrantsManagement_IsCookieOnly(t *testing.T) {
	app := oauthGrantsApp()
	cases := []struct{ method, path string }{
		{fiber.MethodGet, "/api/v1/me/oauth-grants"},
		{fiber.MethodDelete, "/api/v1/me/oauth-grants/1"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer fhm_whatever")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (grant management must be cookie-only)", resp.StatusCode)
			}
		})
	}
}
