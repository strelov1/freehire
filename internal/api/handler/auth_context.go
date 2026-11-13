package handler

import (
	"crypto/subtle"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/apiusage"
	"github.com/strelov1/freehire/internal/identity/auth"
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

// countAPIUsage counts a request toward its account's API usage when the gateway
// attributed it to an account (externalCaller) and it carries a Bearer credential, so a
// signed-in visitor browsing the site is never counted. The status is read after the
// chain: an error the error handler has not rendered yet is counted by its code, and a
// panic (recovered further out) as a 500.
func countAPIUsage(counter *apiusage.Counter) fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		userID, _ := c.Locals(localsExternalCallerID).(int64)
		if userID == 0 || auth.PresentedBearer(c) == "" {
			return c.Next()
		}
		status := fiber.StatusInternalServerError
		defer func() { counter.Record(userID, status >= 400, time.Now()) }()
		err = c.Next()
		status = c.Response().StatusCode()
		if err != nil {
			status = fiber.StatusInternalServerError
			if fe, ok := err.(*fiber.Error); ok {
				status = fe.Code
			}
		}
		return err
	}
}
