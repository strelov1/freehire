package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/strelov1/freehire/internal/identity/apiusage"
	"github.com/strelov1/freehire/internal/platform/db"
)

type usageSink struct{ requests, errors int64 }

func (s *usageSink) AddAPIUsage(_ context.Context, arg db.AddAPIUsageParams) error {
	for i := range arg.UserIds {
		s.requests += arg.Requests[i]
		s.errors += arg.Errors[i]
	}
	return nil
}

// Only a gateway-attributed request carrying a Bearer credential counts; a 4xx, whether
// written or returned as an error, counts as an error too.
func TestCountAPIUsage(t *testing.T) {
	sink := &usageSink{}
	counter := apiusage.New(sink)
	app := fiber.New()
	app.Use(recover.New()) // outside the counter, as in cmd/server
	app.Use(func(c *fiber.Ctx) error {
		if c.Get("X-Test-User") != "" {
			c.Locals(localsExternalCallerID, int64(7))
		}
		return c.Next()
	})
	app.Use(countAPIUsage(counter))
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/missing", func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusNotFound, "no") })
	app.Get("/panic", func(c *fiber.Ctx) error { panic("boom") })

	for _, tc := range []struct {
		path, user, auth string
	}{
		{"/ok", "1", "Bearer fhk_x"},      // counted
		{"/ok", "1", "bearer fhk_x"},      // counted: the scheme is case-insensitive
		{"/missing", "1", "Bearer fhk_x"}, // counted, error
		{"/panic", "1", "Bearer fhk_x"},   // counted, error: recovered further out
		{"/ok", "", "Bearer fhk_x"},       // not attributed
		{"/ok", "1", ""},                  // a signed-in visitor, no Bearer
	} {
		req := httptest.NewRequestWithContext(context.Background(), "GET", tc.path, nil)
		if tc.user != "" {
			req.Header.Set("X-Test-User", tc.user)
		}
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if err := counter.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sink.requests != 4 || sink.errors != 2 {
		t.Fatalf("got %d requests / %d errors, want 4 / 2", sink.requests, sink.errors)
	}
}
