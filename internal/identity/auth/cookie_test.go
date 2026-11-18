package auth

import (
	"context"
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
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
	if err != nil {
		t.Fatalf("test request failed: %v", err)
	}
	defer resp.Body.Close()
	return resp.Header.Get("Set-Cookie")
}

// A non-empty domain must appear as the cookie's Domain attribute so the session
// is shared across freehire.me and apply.freehire.me (unified SSO).
func TestSetTokenCookieSetsDomain(t *testing.T) {
	sc := setCookieHeader(t, ".freehire.me")
	if !strings.Contains(sc, CookieName+"=tok") {
		t.Fatalf("cookie value missing in %q", sc)
	}
	if !strings.Contains(strings.ToLower(sc), "domain=.freehire.me") {
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
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
	if err != nil {
		t.Fatalf("test request failed: %v", err)
	}
	defer resp.Body.Close()
	return resp.Header.Values("Set-Cookie")
}

// Logout during the cookie-domain migration must clear BOTH the configured
// `.freehire.me` cookie and any leftover host-only cookie — otherwise a user
// still holding an old host-only cookie can never log out.
func TestClearTokenCookieClearsBothScopesWithDomain(t *testing.T) {
	scs := clearCookieHeaders(t, ".freehire.me")
	if len(scs) != 2 {
		t.Fatalf("expected 2 Set-Cookie headers (domain + host-only), got %d: %q", len(scs), scs)
	}
	var haveDomain, haveHostOnly bool
	for _, sc := range scs {
		if !strings.Contains(sc, CookieName+"=;") {
			t.Fatalf("expected an expiring clear, got %q", sc)
		}
		low := strings.ToLower(sc)
		if strings.Contains(low, "domain=.freehire.me") {
			haveDomain = true
		} else if !strings.Contains(low, "domain=") {
			haveHostOnly = true
		}
	}
	if !haveDomain || !haveHostOnly {
		t.Fatalf("expected both a .freehire.me clear and a host-only clear, got %q", scs)
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

func TestBrowserOriginsIncludesExactServedHosts(t *testing.T) {
	got := BrowserOrigins("http://localhost:5173/", []string{".freehire.me"}, " Apply.Freehire.Me ", "", "*.freehire.me", "https://invalid.example")
	want := []string{"http://localhost:5173", "https://freehire.me", "https://apply.freehire.me"}
	if len(got) != len(want) {
		t.Fatalf("origins = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("origin %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// hasAttrs fails unless every wanted attribute is present in the Set-Cookie header.
func hasAttrs(t *testing.T, what, header string, want ...string) {
	t.Helper()
	low := strings.ToLower(header)
	for _, w := range want {
		if !strings.Contains(low, w) {
			t.Errorf("%s %q lacks %q", what, header, w)
		}
	}
}

// The session JWT cookie's only XSS/CSRF defence is HttpOnly + SameSite=Lax (+ Secure
// in prod); pin them on the set and on both logout clears.
func TestSessionCookieAttributes(t *testing.T) {
	hasAttrs(t, "session cookie", setCookieHeader(t, ".freehire.me"), "httponly", "samesite=lax", "secure", "path=/")
	for _, sc := range clearCookieHeaders(t, ".freehire.me") {
		hasAttrs(t, "clear cookie", sc, "httponly", "samesite=lax", "secure", "path=/")
	}

	// Dev http path: no Secure on the set or the clear.
	app := fiber.New()
	app.Get("/set", func(c *fiber.Ctx) error {
		SetTokenCookie(c, "tok", time.Hour, false, "")
		return nil
	})
	app.Get("/clear", func(c *fiber.Ctx) error {
		ClearTokenCookie(c, false, ".freehire.me")
		return nil
	})
	for _, path := range []string{"/set", "/clear"} {
		resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", path, nil))
		if err != nil {
			t.Fatalf("test request failed: %v", err)
		}
		_ = resp.Body.Close()
		for _, sc := range resp.Header.Values("Set-Cookie") {
			hasAttrs(t, "insecure "+path, sc, "httponly", "samesite=lax")
			if strings.Contains(strings.ToLower(sc), "secure") {
				t.Errorf("insecure %s cookie %q carries Secure", path, sc)
			}
		}
	}
}
