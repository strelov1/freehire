package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestContractMethodNotAllowedUsesTheMatchedResource(t *testing.T) {
	for _, tc := range []struct{ path, allow string }{
		{"/api/v1/me/watchlist/import", "POST"},
		{"/api/v1/ME/WATCHLIST/IMPORT/", "POST"},
		{"/api/v1/me/watchlist/acme", "GET, HEAD, PUT, DELETE"},
		{"/api/v1/me/watchlist", "GET, HEAD"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			app := watchApp(&watchQueries{}, 7)
			resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodOptions, tc.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusMethodNotAllowed {
				t.Fatalf("status=%d, want 405", resp.StatusCode)
			}
			if got := resp.Header.Get(fiber.HeaderAllow); got != tc.allow {
				t.Errorf("Allow=%q, want %q", got, tc.allow)
			}
		})
	}
}

func TestContractMethodNotAllowedLeavesHandwrittenRoutesAlone(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/handwritten", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodOptions, "/api/v1/handwritten", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusMethodNotAllowed || resp.Header.Get(fiber.HeaderAllow) != "POST" {
		t.Errorf("status=%d Allow=%q, want 405 POST", resp.StatusCode, resp.Header.Get(fiber.HeaderAllow))
	}
}
