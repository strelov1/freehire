package handler

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// localsAutoApplySystemCaller marks a request as authenticated by the shared auto-apply
// orchestrator secret (openspec/changes/auto-apply-inngest-orchestration) rather than by
// proving ownership of the entry it names. It is a request-scoped flag, not a
// auth.LocalsUserID: this caller's user id comes from the queue entry itself
// (resolveAutoApplyEntry), never from who it authenticated as.
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

// autoApplyBearerToken extracts an `Authorization: Bearer <token>` credential. A small
// local copy of the same parsing auth.bearerToken already does (unexported there, and Go
// has no way to share an unexported helper across packages) — this package has no other
// caller for it.
func autoApplyBearerToken(c *fiber.Ctx) (string, bool) {
	const prefix = "Bearer "
	h := c.Get(fiber.HeaderAuthorization)
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):]), true
	}
	return "", false
}
