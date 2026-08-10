package ratelimit

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
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
func KeyByUserOrIP(prefix string) func(*fiber.Ctx) string {
	return func(c *fiber.Ctx) string {
		if id, ok := auth.UserID(c); ok {
			return prefix + ":user:" + strconv.FormatInt(id, 10)
		}
		return prefix + ":ip:" + c.IP()
	}
}
