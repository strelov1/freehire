package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// An unrecognized link makes the server fetch an attacker-chosen URL, so the endpoint is
// an outbound-fetch amplifier unless it is bounded. The bound must be keyed on the
// authenticated user: keying on client IP would let a rotating proxy pool lift it.
func TestContributionLimiter_IsKeyedOnTheUser(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// Stand in for the auth middleware: the user id comes from a header so one test
	// can act as two users from the same address.
	app.Post("/contrib", func(c *fiber.Ctx) error {
		id := int64(1)
		if c.Get("X-Test-User") == "2" {
			id = 2
		}
		c.Locals("auth.userID", id)
		return c.Next()
	}, contributionLimiter(newTestThrottler(t)), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusCreated) })

	post := func(user string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/contrib", nil)
		req.Header.Set("X-Test-User", user)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	var refused int
	for i := 0; i < contributionsPerHour+5; i++ {
		if post("1") == fiber.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatalf("user 1 sent %d submissions without being throttled", contributionsPerHour+5)
	}

	// A different user is unaffected by the first one's exhausted budget.
	if got := post("2"); got == fiber.StatusTooManyRequests {
		t.Error("a second user was throttled by the first user's budget — the key is not the user")
	}
}
