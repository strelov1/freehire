package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

// meJobsApp mounts the my-jobs listing behind RequireAuth on a handler with no
// DB. The cases below (auth gate, filter validation) reject before any query
// runs, so the nil queries is never dereferenced; the DB-backed listing contract
// is covered by the integration test.
func meJobsApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &trackingHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/tracking", auth.RequireAuth(iss, testVersions), h.ListTrackedJobs)
	return app, token
}

func getMeTracking(t *testing.T, app *fiber.App, path, token string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestListMyJobs_RequiresAuth(t *testing.T) {
	app, _ := meJobsApp(t)
	if got := getMeTracking(t, app, "/me/tracking", ""); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestListMyJobs_UnknownFilter(t *testing.T) {
	app, token := meJobsApp(t)
	if got := getMeTracking(t, app, "/me/tracking?filter=bogus", token); got != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}
