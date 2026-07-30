package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/strelov1/freehire/internal/auth"
)

// matchAnalysesPerHour bounds how many fit analyses one user may run per hour. Running one
// drives a three-stage LLM prompt chain, so the endpoint spends real money per call — and
// only a NEW (user, job) analysis costs an AI credit. Recomputing one already cached is free
// by design, which leaves the credit budget bounding the first run of each job and nothing
// bounding the re-runs.
//
// Thirty cannot be reached by legitimate use. The free tier grants 20 points a month and a
// new analysis costs 1 (CREDITS_MONTHLY_GRANT / CREDITS_COST_MATCH), so the credit budget
// already bounds first runs well below this in a whole month — including the SPA's automatic
// run on a job page with no cached analysis. What is left is deliberate re-running of an
// analysis already cached, which costs nothing and is the only way to get here.
const matchAnalysesPerHour = 30

// matchAnalysisLimiter throttles the two fit-analysis routes that run the chain (POST and the
// SSE stream, plus their deprecated `fit` aliases) per authenticated user. The cached GET is
// deliberately not throttled: it reads the stored analysis and recomputes only the
// hard-constraint ceiling, so it costs a query, not a model call.
//
// Keying on the user rather than c.IP() is the point, exactly as in contributionLimiter: the
// caller is already authenticated, and an IP key would be lifted by any rotating proxy pool.
// It must be mounted AFTER the auth middleware so the user id is resolved; a request that
// somehow arrives unauthenticated falls back to the address, which is stricter, not looser.
func matchAnalysisLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        matchAnalysesPerHour,
		Expiration: time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := auth.UserID(c); ok {
				return "match:user:" + strconv.FormatInt(id, 10)
			}
			return "match:ip:" + c.IP()
		},
	})
}
