package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

// notificationsApp mounts the notification-center routes behind RequireAuth
// (cookie-only) on a handler with no DB. The cookie-gate cases below reject
// before any query runs, so the nil queries is never dereferenced. The
// DB-backed list/pagination/mark-read behavior is covered by the integration
// tests — this package's `authHandlers.queries` is a concrete `*db.Queries`,
// not an injectable interface, so there is no fake-friendly seam for a unit
// test to exercise those paths, same as the push-token handlers above.
func notificationsApp() (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/notifications", auth.RequireAuth(iss, testVersions), h.GetNotifications)
	app.Get("/api/v1/me/notifications/:id", auth.RequireAuth(iss, testVersions), h.GetNotification)
	app.Post("/api/v1/me/notifications/:id/read", auth.RequireAuth(iss, testVersions), h.MarkNotificationRead)
	app.Post("/api/v1/me/notifications/read-all", auth.RequireAuth(iss, testVersions), h.MarkAllNotificationsRead)
	return app, iss
}

// The notification center is cookie-only: a request with an API key (or
// nothing) but no session cookie must be rejected, matching every other
// personal /me/* surface that isn't explicitly opened to the agent harness.
func TestNotificationsManagement_IsCookieOnly(t *testing.T) {
	app, _ := notificationsApp()
	cases := []struct {
		name, method, path string
		bearer             bool
	}{
		{"list, no credential", fiber.MethodGet, "/api/v1/me/notifications", false},
		{"list, bearer only", fiber.MethodGet, "/api/v1/me/notifications", true},
		{"get one, bearer only", fiber.MethodGet, "/api/v1/me/notifications/1", true},
		{"mark read, bearer only", fiber.MethodPost, "/api/v1/me/notifications/1/read", true},
		{"mark all read, bearer only", fiber.MethodPost, "/api/v1/me/notifications/read-all", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil)
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer fhk_whatever")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (notification-center access must be cookie-only)", resp.StatusCode)
			}
		})
	}
}
