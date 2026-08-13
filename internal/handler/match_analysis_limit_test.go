package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Running the fit analysis spends real LLM budget, and only a NEW (user, job) analysis costs
// an AI credit — a recompute of one already cached is free by design. Free plus unbounded is
// the problem: nothing else limits how often a signed-in caller may re-run the three-stage
// chain. The bound must be keyed on the authenticated user, not the address, for the same
// reason the contribution limiter is: a rotating proxy pool lifts an IP key.
func TestMatchAnalysisLimiter_IsKeyedOnTheUser(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// Stand in for the auth middleware: the user id comes from a header so one test can act
	// as two users from the same address.
	app.Post("/analysis", func(c *fiber.Ctx) error {
		id := int64(1)
		if c.Get("X-Test-User") == "2" {
			id = 2
		}
		c.Locals("auth.userID", id)
		return c.Next()
	}, matchAnalysisLimiter(newTestThrottler(t)), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	post := func(user string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/analysis", nil)
		req.Header.Set("X-Test-User", user)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	var refused int
	for i := 0; i < matchAnalysesPerHour+5; i++ {
		if post("1") == fiber.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatalf("user 1 ran %d analyses without being throttled", matchAnalysesPerHour+5)
	}

	// A different user is unaffected by the first one's exhausted budget.
	if got := post("2"); got == fiber.StatusTooManyRequests {
		t.Error("a second user was throttled by the first user's budget — the key is not the user")
	}
}
