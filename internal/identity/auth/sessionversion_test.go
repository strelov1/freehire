package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestSessionVersion pins the accessor contract: the value resolveSession stored
// is what a handler reads, and a request that never went through session
// middleware reads an error — never a silent zero that could pass for a
// generation (generations start at 1, but the fail-closed shape matters: the
// accessor must not hand out a value the middleware never proved).
func TestSessionVersion(t *testing.T) {
	app := fiber.New()
	var got int32
	var gotErr error
	app.Get("/", func(c *fiber.Ctx) error {
		got, gotErr = SessionVersion(c)
		return nil
	})

	t.Run("no session middleware, no version", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/", nil)
		resp, testErr := app.Test(req)
		if testErr != nil {
			t.Fatalf("app.Test: %v", testErr)
		}
		defer resp.Body.Close()
		if !errors.Is(gotErr, ErrNoSession) {
			t.Errorf("err = %v, want ErrNoSession", gotErr)
		}
		if got != 0 {
			t.Errorf("version = %d, want 0", got)
		}
	})

	t.Run("the value the middleware stored is what the handler reads", func(t *testing.T) {
		// Fresh app so the first subtest's read cannot leak into this one.
		app := fiber.New()
		var v int32
		var err error
		app.Get("/", func(c *fiber.Ctx) error {
			c.Locals(localsSessionVersion, int32(7)) // what resolveSession stores
			v, err = SessionVersion(c)
			return nil
		})
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/", nil)
		resp, testErr := app.Test(req)
		if testErr != nil {
			t.Fatalf("app.Test: %v", testErr)
		}
		defer resp.Body.Close()
		if err != nil {
			t.Fatalf("SessionVersion: %v", err)
		}
		if v != 7 {
			t.Errorf("version = %d, want 7", v)
		}
	})
}
