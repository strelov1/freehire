package ratelimit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func TestKeyByIP_IncludesPrefix(t *testing.T) {
	app := fiber.New()
	app.Get("/probe", func(c *fiber.Ctx) error {
		key := KeyByIP("login")(c)
		return c.SendString(key)
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(t, resp)
	if !strings.HasPrefix(body, "login:") {
		t.Errorf("key = %q, want prefix %q", body, "login:")
	}
}

func TestKeyByIP_DifferentPrefixesDoNotCollide(t *testing.T) {
	app := fiber.New()
	app.Get("/a", func(c *fiber.Ctx) error { return c.SendString(KeyByIP("routeA")(c)) })
	app.Get("/b", func(c *fiber.Ctx) error { return c.SendString(KeyByIP("routeB")(c)) })

	respA, _ := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/a", nil))
	defer respA.Body.Close()
	respB, _ := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/b", nil))
	defer respB.Body.Close()

	keyA := readBody(t, respA)
	keyB := readBody(t, respB)
	if keyA == keyB {
		t.Errorf("expected different keys for different route prefixes, both were %q", keyA)
	}
}

func TestKeyByUserOrIP_UsesUserIDWhenAuthenticated(t *testing.T) {
	app := fiber.New()
	app.Get("/probe", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(42))
		return c.SendString(KeyByUserOrIP("photo")(c))
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if got, want := readBody(t, resp), "photo:user:42"; got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
}

func TestKeyByUserOrIP_FallsBackToIPWhenUnauthenticated(t *testing.T) {
	app := fiber.New()
	app.Get("/probe", func(c *fiber.Ctx) error {
		return c.SendString(KeyByUserOrIP("photo")(c))
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(t, resp)
	if !strings.HasPrefix(body, "photo:ip:") {
		t.Errorf("key = %q, want prefix %q", body, "photo:ip:")
	}
}

func TestKeyByUserOrIP_DifferentPrefixesDoNotCollideForSameUser(t *testing.T) {
	app := fiber.New()
	app.Get("/a", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		return c.SendString(KeyByUserOrIP("photo")(c))
	})
	app.Get("/b", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		return c.SendString(KeyByUserOrIP("contribution")(c))
	})

	respA, _ := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/a", nil))
	defer respA.Body.Close()
	respB, _ := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/b", nil))
	defer respB.Body.Close()

	keyA := readBody(t, respA)
	keyB := readBody(t, respB)
	if keyA == keyB {
		t.Errorf("same user, different routes must not collide, both were %q", keyA)
	}
}
