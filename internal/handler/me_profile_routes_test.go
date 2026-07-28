package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// namedGate answers with its own name instead of calling the handler, so a request
// reveals which gate a route was registered behind.
//
// TODO: fold into the identical probe in cv_routes_test.go once #1172 lands — both
// branches grew one independently.
func namedGate(name string) fiber.Handler {
	return func(c *fiber.Ctx) error { return c.SendString(name) }
}

// TestProfileRegister_ReadTakesAKeyAndWritesDoNot pins the gate behind each profile
// route against the real register().
//
// The read is cvKey (cookie, full key, or the narrow `cv` key) so both agents can reach
// it: the assistant, which authenticates with the caller's session token, and the CV
// tailoring agent, whose minted key is `cv`-scoped and would be 403'd by key. The writes
// stay cookie-only — a key that leaks out of an agent's environment must not be able to
// rewrite or clear somebody's profile.
func TestProfileRegister_ReadTakesAKeyAndWritesDoNot(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&profileHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	for _, tc := range []struct {
		method, want string
	}{
		{http.MethodGet, "cvKey"},
		{http.MethodPut, "cookie"},
		{http.MethodDelete, "cookie"},
	} {
		resp, err := app.Test(httptest.NewRequest(tc.method, "/api/v1/me/profile", nil))
		if err != nil {
			t.Fatalf("%s: %v", tc.method, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if got := string(body); got != tc.want {
			t.Errorf("%s /me/profile is gated by %q, want %q", tc.method, got, tc.want)
		}
	}
}
