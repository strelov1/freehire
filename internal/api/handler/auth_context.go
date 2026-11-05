package handler

import (
	"crypto/subtle"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

const localsExternalCallerID = "external_caller_user_id"

// externalCaller accepts trusted attribution metadata without granting application permissions.
func externalCaller(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == "/internal/auth/introspect" {
			return c.Next()
		}
		if secret != "" && subtle.ConstantTimeCompare([]byte(c.Get(authIntrospectionSecretHeader)), []byte(secret)) == 1 {
			if id, err := strconv.ParseInt(c.Get("X-FH-Auth-User"), 10, 64); err == nil && id > 0 {
				c.Locals(localsExternalCallerID, id)
			}
		}
		c.Request().Header.Del(authIntrospectionSecretHeader)
		c.Request().Header.Del("X-FH-Auth-User")
		return c.Next()
	}
}
