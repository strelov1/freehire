package handler

import (
	"crypto/subtle"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
)

// localsAutoApplySystemCaller marks a request as authenticated by the shared auto-apply
// orchestrator secret (openspec/changes/auto-apply-inngest-orchestration) rather than by
// proving ownership of the entry it names. It is a request-scoped flag, not a
// auth.LocalsUserID: this caller's user id comes from the queue entry itself
// (resolveAutoApplyEntry), never from who it authenticated as.
const localsAutoApplySystemCaller = "auto_apply.system_caller"

// isAutoApplySystemCaller reports whether this request authenticated via the shared
// orchestrator secret rather than as the entry's own owner.
func isAutoApplySystemCaller(c *fiber.Ctx) bool {
	v, _ := c.Locals(localsAutoApplySystemCaller).(bool)
	return v
}

// autoApplyOrchestratorRequestsPerHour bounds the whole deployment's own budget for
// system-caller requests on the two auto-apply routes — not per-user, not per-IP, because
// this caller has no such identity of its own. It exists as a compensating control for the
// one property the shared secret cannot offer (per-caller scoping): a leaked secret, or a
// bug that loops these calls, is bounded to one shared budget rather than unbounded. Sized
// generously above any plausible early-stage volume (each entry costs at most one tailor
// call and one review call) — tighten once real traffic is observed.
const autoApplyOrchestratorRequestsPerHour = 500

// autoApplyOrchestratorLimiterKey is the fixed rate-limit key for every system-caller
// request on the two auto-apply routes: one shared budget for the one trusted process,
// never split per-user or per-IP.
func autoApplyOrchestratorLimiterKey(*fiber.Ctx) string { return "auto-apply-orchestrator" }

// autoApplyOrchestratorGate is the auth gate for POST /me/auto-apply/:queueId/{tailor,review}.
// A Bearer token equal (constant-time) to the configured shared secret marks the request as
// the trusted orchestrator process, applies the process-wide rate limit, and proceeds without
// any per-user identity — see resolveAutoApplyEntry, which is what turns this into an actual
// user for the rest of the request. Anything else — no secret configured, no Bearer token, or
// a token that does not match — falls through unchanged to humanAuth (the ordinary keyAuth
// resolution: session cookie or a live api_keys row).
//
// An empty secret disables the fallback path entirely: the routes then behave exactly as
// they did under plain keyAuth, with no new code path reachable.
//
// Uses ratelimit.MiddlewareIgnoringTrustedPeers, not ratelimit.Middleware: the orchestrator
// reaches hire over loopback (its own default HireBaseURL is http://127.0.0.1:<port>, the
// real host-2 deploy shape), and plain Middleware exempts every loopback peer from rate
// limiting entirely — which would silently void the one compensating control this shared
// secret has for having no per-caller scoping of its own.
func autoApplyOrchestratorGate(secret string, humanAuth fiber.Handler, throttler ratelimit.Throttler) fiber.Handler {
	limiter := ratelimit.MiddlewareIgnoringTrustedPeers(throttler, autoApplyOrchestratorLimiterKey, autoApplyOrchestratorRequestsPerHour, time.Hour)
	return func(c *fiber.Ctx) error {
		if secret != "" {
			if tok, ok := autoApplyBearerToken(c); ok && subtle.ConstantTimeCompare([]byte(tok), []byte(secret)) == 1 {
				c.Locals(localsAutoApplySystemCaller, true)
				return limiter(c)
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
