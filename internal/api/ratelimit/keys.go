package ratelimit

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// KeyByIP returns a key function that identifies a caller solely by IP address,
// namespaced under prefix so it cannot collide with any other route's keys.
func KeyByIP(prefix string) func(*fiber.Ctx) string {
	return func(c *fiber.Ctx) string {
		return prefix + ":" + c.IP()
	}
}

// KeyByUserOrIP returns a key function that identifies an authenticated caller by
// user id, falling back to IP address when the request carries no authenticated
// user. Both forms are namespaced under prefix so it cannot collide with any
// other route's keys.
//
// It reads the user id an authentication gate left in locals, so it is only
// meaningful on a route that mounts the limiter AFTER that gate. Mounted before
// one — or on a route that has none — the user branch is unreachable and the key
// silently degrades to the IP branch, which is worse than KeyByIP rather than
// equal to it: the ":ip:" discriminator claims a distinction the route cannot
// make. Three public read limiters shipped that way. Use KeyByIP when the gate is
// not ahead of you; see internal/api/handler/AGENTS.md.
func KeyByUserOrIP(prefix string) func(*fiber.Ctx) string {
	return func(c *fiber.Ctx) string {
		if id, ok := auth.UserID(c); ok {
			return prefix + ":user:" + strconv.FormatInt(id, 10)
		}
		return prefix + ":ip:" + c.IP()
	}
}
