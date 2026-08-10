package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ratelimit"
)

// mailRecallsPerHour bounds how many mailbox sweeps one caller may run per hour.
//
// This endpoint is unmetered by design — no credit is debited, because a price set before
// the spend distribution is known is a guess — and each press sends up to forty message
// bodies to the model in one call. With `LLM_USER_MAX_BUDGET` deliberately unset in
// production, nothing downstream bounds it either, so this limiter is the whole of the
// cost gate. It is mounted on a route that accepts a full-scope API key, which means the
// caller need not be a browser to press the button repeatedly.
//
// Twenty cannot be reached by legitimate use: the sweep is a per-application act, a second
// press on the same application finds nothing new (its first run's proposals are no longer
// unattached), and twenty distinct applications swept in one hour is already a busy day of
// deliberate work.
const mailRecallsPerHour = 20

// mailRecallLimiter throttles the sweep per authenticated caller.
//
// Keyed on the user rather than c.IP() for the same reason as matchAnalysisLimiter: the
// caller is already authenticated, and an IP key would be lifted by any rotating proxy
// pool. It must be mounted AFTER the auth middleware so the id is resolved; a request that
// somehow arrives unauthenticated falls back to the address, which is stricter, not looser.
func mailRecallLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByUserOrIP("mailrecall"), mailRecallsPerHour, time.Hour)
}
