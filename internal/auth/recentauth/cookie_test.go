package recentauth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestRecentAuthCookieAttributes(t *testing.T) {
	app := fiber.New()
	app.Get("/set", func(c *fiber.Ctx) error {
		SetCookie(c, "proof", time.Now().Add(time.Minute), true, "freehire.me")
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/set", nil))
	if err != nil {
		t.Fatal(err)
	}
	header := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	lower := strings.ToLower(header)
	for _, want := range []string{CookieName + "=proof", "path=/", "domain=freehire.me", "httponly", "secure", "samesite=lax"} {
		if !strings.Contains(lower, want) {
			t.Errorf("Set-Cookie %q missing %q", header, want)
		}
	}
}

func TestClearRecentAuthCookieExpiresSameName(t *testing.T) {
	app := fiber.New()
	app.Get("/clear", func(c *fiber.Ctx) error {
		ClearCookie(c, false, "")
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/clear", nil))
	if err != nil {
		t.Fatal(err)
	}
	header := strings.Join(resp.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(header, CookieName+"=") || !strings.Contains(strings.ToLower(header), "expires=") {
		t.Fatalf("clear cookie is incomplete: %q", header)
	}
}
