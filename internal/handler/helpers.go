package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

// requireUserID returns the id the auth middleware stored on the request. The
// middleware (RequireAuth / RequireAuthOrKey) guarantees it for every route it
// guards; the error branch is a defensive 401 for a handler wired without that
// middleware.
func requireUserID(c *fiber.Ctx) (int64, error) {
	id, ok := auth.UserID(c)
	if !ok {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	return id, nil
}

// pathID parses the ":id" route param as an int64, returning a 400 on a malformed
// value. It centralizes the parse + the int64 conversion the handlers repeat.
func pathID(c *fiber.Ctx) (int64, error) {
	id, err := c.ParamsInt("id")
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	return int64(id), nil
}
