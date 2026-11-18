package handler

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// /token errors, invalid_target included, must not be cached (RFC 6749 5.1/5.2).
// The route's no-store middleware runs before the handler, so it covers the
// error paths too; this drives the real route tree to keep it that way.
func TestOAuthToken_InvalidTargetIsNotCacheable(t *testing.T) {
	h := &authHandlers{frontendOrigin: "https://freehire.me"}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	passthrough := func(c *fiber.Ctx) error { return c.Next() }
	h.register(app.Group("/api/v1"), middleware{cookie: passthrough, optionalCookie: passthrough}, newContract(contractHandlers{authHandlers: h}))

	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {"c"}, "client_id": {"cid"},
		"redirect_uri": {"http://127.0.0.1:1/cb"}, "code_verifier": {"v"},
		"resource": {"https://evil.example/api/v1/mcp"},
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}
