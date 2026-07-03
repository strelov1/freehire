package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// setCookieHeader runs a handler that writes the auth cookie with the given
// domain and returns the resulting Set-Cookie header.
func setCookieHeader(t *testing.T, domain string) string {
	t.Helper()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		SetTokenCookie(c, "tok", time.Hour, true, domain)
		return c.SendStatus(fiber.StatusOK)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("test request failed: %v", err)
	}
	return resp.Header.Get("Set-Cookie")
}

// A non-empty domain must appear as the cookie's Domain attribute so the session
// is shared across freehire.dev and apply.freehire.dev (unified SSO).
func TestSetTokenCookieSetsDomain(t *testing.T) {
	sc := setCookieHeader(t, ".freehire.dev")
	if !strings.Contains(sc, CookieName+"=tok") {
		t.Fatalf("cookie value missing in %q", sc)
	}
	if !strings.Contains(strings.ToLower(sc), "domain=.freehire.dev") {
		t.Fatalf("expected domain attribute, got %q", sc)
	}
}

// An empty domain must omit the Domain attribute entirely (host-only), which is
// what dev on localhost relies on.
func TestSetTokenCookieEmptyDomainOmitsAttribute(t *testing.T) {
	sc := setCookieHeader(t, "")
	if strings.Contains(strings.ToLower(sc), "domain=") {
		t.Fatalf("expected no domain attribute, got %q", sc)
	}
}
