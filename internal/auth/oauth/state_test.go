package oauth

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestNewState_RandomAndURLSafe(t *testing.T) {
	a, err := NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	b, err := NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if a == b {
		t.Error("two states are equal; want random")
	}
	if len(a) < 32 {
		t.Errorf("state %q too short", a)
	}
	for _, r := range a {
		if r == '+' || r == '/' || r == '=' {
			t.Errorf("state %q is not URL-safe", a)
		}
	}
}

func TestSafeReturnPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty falls back to root", "", "/"},
		{"relative path kept", "/jobs/performance-engineer-kyw26tid", "/jobs/performance-engineer-kyw26tid"},
		{"query preserved", "/jobs?remote=true&q=go", "/jobs?remote=true&q=go"},
		{"absolute url rejected", "https://evil.com/phish", "/"},
		{"scheme-relative url rejected", "//evil.com/phish", "/"},
		{"non-rooted path rejected", "jobs/foo", "/"},
		{"backslash trick rejected", "\\evil.com", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeReturnPath(tc.in); got != tc.want {
				t.Errorf("SafeReturnPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStateCookie_SetAndClear(t *testing.T) {
	app := fiber.New()
	app.Get("/set", func(c *fiber.Ctx) error {
		SetStateCookie(c, "abc", false)
		return nil
	})
	app.Get("/clear", func(c *fiber.Ctx) error {
		ClearStateCookie(c, false)
		return nil
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/set", nil))
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	defer resp.Body.Close()
	set := resp.Header.Get("Set-Cookie")
	for _, want := range []string{StateCookieName + "=abc", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(set, want) {
			t.Errorf("set cookie %q missing %q", set, want)
		}
	}

	resp, err = app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/clear", nil))
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	defer resp.Body.Close()
	cleared := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cleared, StateCookieName+"=") {
		t.Errorf("clear cookie %q does not clear %s", cleared, StateCookieName)
	}
}

// Apple's callback arrives as a cross-site POST (response_mode=form_post) —
// browsers never attach a SameSite=Lax cookie to a cross-site POST, only to a
// cross-site top-level GET navigation. A Lax state cookie therefore never
// reaches the server on Apple's callback, so every Apple sign-in fails with a
// false "state mismatch". None is required to survive that POST, and None is
// rejected by browsers unless Secure is also set — so this only applies when
// the deployment is on HTTPS (secure=true); an insecure dev deployment keeps
// Lax, where None would be silently dropped instead of helping.
func TestStateCookie_SameSiteNoneWhenSecure(t *testing.T) {
	app := fiber.New()
	app.Get("/set", func(c *fiber.Ctx) error {
		SetStateCookie(c, "abc", true)
		return nil
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/set", nil))
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	defer resp.Body.Close()
	set := resp.Header.Get("Set-Cookie")
	for _, want := range []string{StateCookieName + "=abc", "HttpOnly", "secure", "SameSite=None"} {
		if !strings.Contains(set, want) {
			t.Errorf("set cookie %q missing %q", set, want)
		}
	}
	if strings.Contains(set, "SameSite=Lax") {
		t.Errorf("set cookie %q still SameSite=Lax when secure=true", set)
	}
}
