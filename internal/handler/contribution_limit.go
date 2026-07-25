package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/strelov1/freehire/internal/auth"
)

// contributionsPerHour bounds how many board links one user may submit per hour. A
// submission whose URL is not a recognized ATS makes the server fetch that URL, so an
// unbounded endpoint is an outbound-fetch amplifier and a timing oracle for public hosts.
// Twenty is far above real use — a human pasting links from a careers page — and far below
// anything useful for abuse.
const contributionsPerHour = 20

// contributionLimiter throttles board submissions per authenticated user. Keying on the
// user rather than c.IP() is the whole point: the caller is already authenticated, and an
// IP key would be lifted by any rotating proxy pool. It must be mounted AFTER the auth
// middleware so the user id is resolved; a request that somehow arrives unauthenticated
// falls back to the address, which is stricter, not looser.
func contributionLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        contributionsPerHour,
		Expiration: time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := auth.UserID(c); ok {
				return "user:" + strconv.FormatInt(id, 10)
			}
			return "ip:" + c.IP()
		},
	})
}
