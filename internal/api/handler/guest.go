package handler

import (
	"github.com/gofiber/fiber/v2"
)

// refuseEncodedBody answers 415 for a request carrying any Content-Encoding. Call it on an
// unauthenticated path BEFORE the body is read.
//
// Fiber's Ctx.Body() (behind BodyParser and c.FormFile) DECOMPRESSES Content-Encoding,
// chaining up to three layers with no ceiling on the expanded size (see billing.go's
// webhookFor). The 8MB BodyLimit bounds only the wire body, so without an account one
// request is an unbounded allocation that a per-IP limiter does not bound.
//
// Any non-empty Content-Encoding is refused, with no allow-list: nobody legitimately gzips
// a CV upload or a search description. The shared readers stay untouched so signed-in
// gzip clients keep working.
func refuseEncodedBody(c *fiber.Ctx) error {
	if c.Get(fiber.HeaderContentEncoding) != "" {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "encoded request bodies are not accepted")
	}
	return nil
}
