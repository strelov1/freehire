package handler

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// localsAutoApplySystemCaller marks a request as authenticated by the shared orchestrator
// secret rather than by owning the entry it names. A flag, not an auth.LocalsUserID: the
// user id comes from the queue entry (resolveAutoApplyEntry), never from the credential.
const localsAutoApplySystemCaller = "auto_apply.system_caller"

// isAutoApplySystemCaller reports whether this request authenticated via the shared
// orchestrator secret rather than as the entry's own owner.
func isAutoApplySystemCaller(ctx context.Context) bool {
	v, _ := ctx.Value(localsAutoApplySystemCaller).(bool)
	return v
}

// autoApplyOrchestratorGate accepts the orchestrator secret or the caller's ordinary credentials.
func autoApplyOrchestratorGate(secret string, humanAuth fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if secret != "" {
			if tok, ok := autoApplyBearerToken(c); ok && subtle.ConstantTimeCompare([]byte(tok), []byte(secret)) == 1 {
				c.Locals(localsAutoApplySystemCaller, true)
				return c.Next()
			}
		}
		return humanAuth(c)
	}
}

// autoApplyBearerToken extracts an `Authorization: Bearer <token>` credential; a local copy
// of the unexported auth.bearerToken.
func autoApplyBearerToken(c *fiber.Ctx) (string, bool) {
	const prefix = "Bearer "
	h := c.Get(fiber.HeaderAuthorization)
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):]), true
	}
	return "", false
}
