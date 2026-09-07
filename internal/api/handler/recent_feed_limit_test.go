package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// This is the first public, unauthenticated, indefinitely-held connection endpoint
// in the API — a reconnect loop or a burst of parallel opens costs a live
// goroutine, ticker, and Broadcaster subscription per connection, with no
// session/credential to key a stricter limit on. IP is what's left.
func TestRecentFeedLimiter_ThrottlesRepeatedConnectsFromOneIP(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/feed/recent", recentFeedLimiter(newTestThrottler(t)), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	connect := func() int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/feed/recent", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	var refused int
	for i := 0; i < recentFeedConnectsPerMinute+5; i++ {
		if connect() == fiber.StatusTooManyRequests {
			refused++
		}
	}
	if refused == 0 {
		t.Fatalf("sent %d connects without being throttled", recentFeedConnectsPerMinute+5)
	}
}
