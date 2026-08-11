package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

// pushTokensApp mounts the push-token routes behind RequireAuth (cookie-only)
// on a handler with no DB. The cookie-gate cases below reject before any
// query runs, so the nil queries is never dereferenced. The DB/Expo-backed
// paths are covered by the integration tests.
func pushTokensApp() *fiber.App {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{}
	app := fiber.New()
	app.Post("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.RegisterPushToken)
	app.Delete("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.UnregisterPushToken)
	app.Post("/api/v1/me/push-tokens/test", auth.RequireAuth(iss, testVersions), h.TestPushToken)
	return app
}

// Push-token management is cookie-only: a request with an API key (or
// nothing) but no session cookie must be rejected — the same posture as
// key management, since these endpoints let a caller redirect notifications
// to a device.
func TestPushTokensManagement_IsCookieOnly(t *testing.T) {
	app := pushTokensApp()
	cases := []struct {
		name, method, path string
		bearer             bool
	}{
		{"register, no credential", fiber.MethodPost, "/api/v1/me/push-tokens", false},
		{"register, bearer only", fiber.MethodPost, "/api/v1/me/push-tokens", true},
		{"unregister, bearer only", fiber.MethodDelete, "/api/v1/me/push-tokens", true},
		{"test, bearer only", fiber.MethodPost, "/api/v1/me/push-tokens/test", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer fhk_whatever")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (push-token management must be cookie-only)", resp.StatusCode)
			}
		})
	}
}
