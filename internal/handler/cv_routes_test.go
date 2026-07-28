package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// gateProbe is a middleware that answers with its own name instead of calling the
// handler, so a request reveals which gate a route was registered behind.
func gateProbe(name string) fiber.Handler {
	return func(c *fiber.Ctx) error { return c.SendString(name) }
}

// TestCVRegister_TailoringRoutesAdmitTheNarrowCVKey pins the gate each CV route is
// registered behind, against the real register() — not a hand-wired app.
//
// The tailoring bootstrap mints a key at the narrow `cv` scope, and mw.key
// (RequireAuthOrKey) admits full-scope keys only, answering anything narrower with
// 403. So every route the tailoring agent drives has to sit on mw.cvKey. The existing
// integration test cannot catch a slip here: it mounts its own app with the scoped
// middleware, asserting the intent rather than the wiring.
func TestCVRegister_TailoringRoutesAdmitTheNarrowCVKey(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&cvHandlers{}).register(api, middleware{
		key:    gateProbe("key"),
		cvKey:  gateProbe("cvKey"),
		cookie: gateProbe("cookie"),
	})

	for _, tc := range []struct {
		method, path, want string
	}{
		// Driven by the tailoring agent's narrow key.
		{http.MethodGet, "/api/v1/me/cvs/1", "cvKey"},
		{http.MethodGet, "/api/v1/me/cvs/1/pdf", "cvKey"},
		{http.MethodPatch, "/api/v1/me/cvs/1", "cvKey"},
		{http.MethodPut, "/api/v1/me/cvs/1/session", "cvKey"},
		{http.MethodGet, "/api/v1/me/cvs/1/tailor-context", "cvKey"},
		// Authoring stays with the browser.
		{http.MethodPut, "/api/v1/me/cvs/1", "cookie"},
		{http.MethodDelete, "/api/v1/me/cvs/1", "cookie"},
		{http.MethodPost, "/api/v1/me/cvs", "cookie"},
		{http.MethodPost, "/api/v1/me/cvs/1/tailor-session", "cookie"},
	} {
		resp, err := app.Test(httptest.NewRequest(tc.method, tc.path, nil))
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if got := string(body); got != tc.want {
			t.Errorf("%s %s is gated by %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}
