package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// memberOrGuestLimiter budgets a route open to both signed-in and signed-out callers with
// two separate budgets: member for a caller the optional gate signed in, guest for one it
// did not. A nil member leaves signed-in callers unbudgeted.
//
// Mounted AFTER mw.optional, so the user id is resolved by the time it asks. The two are
// distinct keys on purpose, which is why this is not ratelimit.KeyByUserOrIP: that folds
// both callers into one limiter with one number, and a guest budget sized for an
// anonymous visitor would then also cap the members — or a member budget would be handed
// to every address.
func memberOrGuestLimiter(member, guest fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if _, ok := auth.UserID(c); ok {
			if member == nil {
				return c.Next()
			}
			return member(c)
		}
		return guest(c)
	}
}

// refuseEncodedBody answers 415 for a request carrying any Content-Encoding. Call it on an
// unauthenticated path BEFORE the body is read.
//
// Fiber's Ctx.Body() (which BodyParser and c.FormFile both read through) inspects
// Content-Encoding and DECOMPRESSES, chaining up to three layers, with no ceiling on the
// decompressed size — see billing.go's webhookFor for the same trap on the app's other
// unauthenticated POST. The server's 8MB BodyLimit only bounds the wire body, not what it
// expands to, so a few compressed megabytes become an unbounded allocation. Behind an
// account that is tolerable; without one, a single request is enough to matter, and a
// per-IP limiter does not bound what one request costs.
//
// No allow-list and no parsing of the header's value: nobody legitimately gzips a CV
// upload or a one-line search description, so any non-empty Content-Encoding is refused.
// The shared readers themselves are left untouched, because a signed-in client that
// gzips today must keep working.
func refuseEncodedBody(c *fiber.Ctx) error {
	if c.Get(fiber.HeaderContentEncoding) != "" {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "encoded request bodies are not accepted")
	}
	return nil
}
