package handler

import (
	"net/url"

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

// queryValues parses the raw request query string into url.Values. The handlers
// parse the raw query (rather than Fiber's key-lowercasing accessors) to preserve
// key case and repeated values; this centralizes that one idiom. A malformed query
// yields whatever ParseQuery salvaged, matching the call sites that ignore its error.
func queryValues(c *fiber.Ctx) url.Values {
	vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
	return vals
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
