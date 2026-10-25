package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// bearerGate admits exactly one credential, standing in for RequireAuthOrKey.
func bearerGate(c *fiber.Ctx) error {
	if c.Get(fiber.HeaderAuthorization) != "Bearer good" {
		return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
	}
	return c.Next()
}

// doorApp mounts the door the way New does — after the app-wide middleware, before
// /api/v1 — with a public read, a gated route and a /me route behind it.
func doorApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	mountSignedInDoor(app, bearerGate)
	api := app.Group("/api/v1")
	api.Get("/jobs/search", func(c *fiber.Ctx) error {
		return c.SendString("search q=" + c.Query("q"))
	})
	api.Get("/keys", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusUnauthorized, "cookie only")
	}, func(c *fiber.Ctx) error { return c.SendString("keys") })
	api.Get("/me/tracking", func(c *fiber.Ctx) error { return c.SendString("tracking") })
	api.Get("/me/api-keys", func(c *fiber.Ctx) error { return c.SendString("api-keys") })
	return app
}

func doorGet(t *testing.T, app *fiber.App, path, auth string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	if auth != "" {
		req.Header.Set(fiber.HeaderAuthorization, auth)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestSignedInDoor pins the one property the Cloudflare exemption for /api/v1/me/*
// rests on: nothing behind the door answers a caller without a real credential, and a
// signed-in caller gets exactly what the ordinary path would have given them.
func TestSignedInDoor(t *testing.T) {
	app := doorApp(t)
	for _, tc := range []struct {
		name, path, auth string
		wantStatus       int
		wantBody         string
	}{
		{"anonymous is refused before the route runs", "/api/v1/me/api/jobs/search?q=go", "", 401, ""},
		{"a junk credential is refused", "/api/v1/me/api/jobs/search?q=go", "Bearer junk", 401, ""},
		{"a credential reaches the ordinary route, query intact", "/api/v1/me/api/jobs/search?q=go", "Bearer good", 200, "search q=go"},
		{"the target route's own gate still runs", "/api/v1/me/api/keys", "Bearer good", 401, ""},
		{"a /me route is reachable through it", "/api/v1/me/api/me/tracking", "Bearer good", 200, "tracking"},
		{"the door does not nest", "/api/v1/me/api/me/api/jobs/search", "Bearer good", 404, ""},
		{"the bare door is not a route", "/api/v1/me/api", "Bearer good", 404, ""},
		// It shares the door's string prefix; the door must step aside, not demand a key.
		{"a sibling path is not the door", "/api/v1/me/api-keys", "", 200, "api-keys"},
		{"the ordinary path is untouched", "/api/v1/jobs/search?q=go", "", 200, "search q=go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := doorGet(t, app, tc.path, tc.auth)
			if status != tc.wantStatus {
				t.Fatalf("GET %s: status %d, want %d (body %q)", tc.path, status, tc.wantStatus, body)
			}
			if tc.wantBody != "" && body != tc.wantBody {
				t.Errorf("GET %s: body %q, want %q", tc.path, body, tc.wantBody)
			}
		})
	}
}
