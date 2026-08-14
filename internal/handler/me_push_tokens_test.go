package handler

import (
	"bytes"
	"context"
	"net/http"
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
func pushTokensApp() (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.RegisterPushToken)
	app.Get("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.ListPushTokens)
	app.Delete("/api/v1/me/push-tokens", auth.RequireAuth(iss, testVersions), h.UnregisterPushToken)
	app.Post("/api/v1/me/push-tokens/test", auth.RequireAuth(iss, testVersions), h.TestPushToken)
	return app, iss
}

// Push-token management is cookie-only: a request with an API key (or
// nothing) but no session cookie must be rejected — the same posture as
// key management, since these endpoints let a caller redirect notifications
// to a device.
func TestPushTokensManagement_IsCookieOnly(t *testing.T) {
	app, _ := pushTokensApp()
	cases := []struct {
		name, method, path string
		bearer             bool
	}{
		{"register, no credential", fiber.MethodPost, "/api/v1/me/push-tokens", false},
		{"register, bearer only", fiber.MethodPost, "/api/v1/me/push-tokens", true},
		{"list, no credential", fiber.MethodGet, "/api/v1/me/push-tokens", false},
		{"list, bearer only", fiber.MethodGet, "/api/v1/me/push-tokens", true},
		{"unregister, bearer only", fiber.MethodDelete, "/api/v1/me/push-tokens", true},
		{"test, bearer only", fiber.MethodPost, "/api/v1/me/push-tokens/test", true},
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
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (push-token management must be cookie-only)", resp.StatusCode)
			}
		})
	}
}

// RegisterPushToken must reject a missing/invalid platform or an empty token
// before ever reaching the database — proven here by a nil h.queries: a test
// that reached the DB call would panic instead of returning 400.
func TestRegisterPushToken_ValidationRejectsBeforeAnyDBCall(t *testing.T) {
	app, iss := pushTokensApp()
	cookie, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue cookie: %v", err)
	}

	cases := []struct {
		name, body string
	}{
		{"missing platform", `{"token":"ExponentPushToken[x]"}`},
		{"invalid platform", `{"token":"ExponentPushToken[x]","platform":"windows"}`},
		{"empty token", `{"token":"","platform":"ios"}`},
		{"whitespace-only token", `{"token":"   ","platform":"ios"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/push-tokens", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

// TestPushToken fans out one outbound Expo push call per registered device on every
// hit, unlike the other push-token routes, so it is the one that needs a rate limit —
// the same posture as photo uploads, JD resolve, mail recall, and speech
// transcription. Keyed on the user, like those siblings, so it can't be lifted by a
// rotating proxy pool.
func TestPushTokenTestLimiter_IsKeyedOnTheUser(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/test", func(c *fiber.Ctx) error {
		id := int64(1)
		if c.Get("X-Test-User") == "2" {
			id = 2
		}
		c.Locals("auth.userID", id)
		return c.Next()
	}, testPushTokenLimiter(newTestThrottler(t)), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	post := func(user string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/test", nil)
		req.Header.Set("X-Test-User", user)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	var refused int
	for i := 0; i < testPushTokensPerHour+5; i++ {
		if post("1") == fiber.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatalf("user 1 sent %d test-push requests without being throttled", testPushTokensPerHour+5)
	}

	// A different user is unaffected by the first one's exhausted budget.
	if got := post("2"); got == fiber.StatusTooManyRequests {
		t.Error("a second user was throttled by the first user's budget — the key is not the user")
	}
}
