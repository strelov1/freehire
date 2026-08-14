package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// MatchText runs a multi-pass skilltag.Parse over a caller-supplied body on every call, and
// its own doc comment says it "powers the browser extension's on-any-page card" — i.e. it can
// fire automatically on ordinary tab switches, not just an explicit user action. Unlike the LLM
// fit-analysis routes it was previously left with no limiter at all. Keyed on the user for the
// same reason matchAnalysisLimiter is: an IP key is lifted by any rotating proxy pool.
func TestMatchTextLimiter_IsKeyedOnTheUser(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/match-text", func(c *fiber.Ctx) error {
		id := int64(1)
		if c.Get("X-Test-User") == "2" {
			id = 2
		}
		c.Locals("auth.userID", id)
		return c.Next()
	}, matchTextLimiter(newTestThrottler(t)), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	post := func(user string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/match-text", nil)
		req.Header.Set("X-Test-User", user)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	var refused int
	for i := 0; i < matchTextPerHour+5; i++ {
		if post("1") == fiber.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatalf("user 1 called MatchText %d times without being throttled", matchTextPerHour+5)
	}

	// A different user is unaffected by the first one's exhausted budget.
	if got := post("2"); got == fiber.StatusTooManyRequests {
		t.Error("a second user was throttled by the first user's budget — the key is not the user")
	}
}
