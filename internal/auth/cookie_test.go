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

// clearCookieHeaders runs a logout clear with the given domain and returns every
// Set-Cookie header it emitted.
func clearCookieHeaders(t *testing.T, domain string) []string {
	t.Helper()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		ClearTokenCookie(c, true, domain)
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("test request failed: %v", err)
	}
	return resp.Header.Values("Set-Cookie")
}

// Logout during the cookie-domain migration must clear BOTH the configured
// `.freehire.dev` cookie and any leftover host-only cookie — otherwise a user
// still holding an old host-only cookie can never log out.
func TestClearTokenCookieClearsBothScopesWithDomain(t *testing.T) {
	scs := clearCookieHeaders(t, ".freehire.dev")
	if len(scs) != 2 {
		t.Fatalf("expected 2 Set-Cookie headers (domain + host-only), got %d: %q", len(scs), scs)
	}
	var haveDomain, haveHostOnly bool
	for _, sc := range scs {
		if !strings.Contains(sc, CookieName+"=;") {
			t.Fatalf("expected an expiring clear, got %q", sc)
		}
		low := strings.ToLower(sc)
		if strings.Contains(low, "domain=.freehire.dev") {
			haveDomain = true
		} else if !strings.Contains(low, "domain=") {
			haveHostOnly = true
		}
	}
	if !haveDomain || !haveHostOnly {
		t.Fatalf("expected both a .freehire.dev clear and a host-only clear, got %q", scs)
	}
}

// With no configured domain (dev), a single host-only clear is enough.
func TestClearTokenCookieEmptyDomainSingle(t *testing.T) {
	scs := clearCookieHeaders(t, "")
	if len(scs) != 1 {
		t.Fatalf("expected 1 Set-Cookie header, got %d: %q", len(scs), scs)
	}
	if strings.Contains(strings.ToLower(scs[0]), "domain=") {
		t.Fatalf("expected host-only clear, got %q", scs[0])
	}
}
