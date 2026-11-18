package handler

import (
	"context"
	"net/url"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// requireUserID returns the id the auth middleware stored on the request. The error
// branch is a defensive 401 for a handler wired without that middleware.
func requireUserID(c *fiber.Ctx) (int64, error) {
	id, ok := auth.UserID(c)
	if !ok {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	return id, nil
}

// requireUserIDFromContext is requireUserID for a contract handler (see contract.go).
func requireUserIDFromContext(ctx context.Context) (int64, error) {
	id, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	return id, nil
}

// queryValues parses the raw query string into url.Values, preserving key case and
// repeated values (unlike Fiber's lowercasing accessors). A malformed query yields what
// ParseQuery salvaged.
func queryValues(c *fiber.Ctx) url.Values {
	vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
	return vals
}

// contractPathID is pathID for a contract operation, whose id param is declared a string so
// a value that is not a number keeps the "invalid id" answer.
func contractPathID(s string) (int64, error) {
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	return int64(id), nil
}
