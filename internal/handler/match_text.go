package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/ratelimit"
	"github.com/strelov1/freehire/internal/skilltag"
)

// matchTextPerHour bounds how many ad-hoc text matches one user may run per hour. Unlike
// the LLM fit-analysis routes this costs no model spend, but it still runs a multi-pass
// skilltag.Parse dictionary/regex scan over a body up to the server's global 8MB limit —
// and MatchText's own doc comment says it "powers the browser extension's on-any-page
// card", so it fires automatically on ordinary tab switches/page loads, not just on
// explicit user action. Sized well above that automatic-trigger rate (loadMatch() has no
// debounce of its own) so normal browsing never trips it, while still bounding a scripted
// or compromised-extension-build loop.
const matchTextPerHour = 120

// matchTextLimiter throttles MatchText per authenticated user, the same
// KeyByUserOrIP-keyed shape as matchAnalysisLimiter and jdURLLimiter — a dedicated
// limiter, not a shared one, since this route costs CPU on every page visit rather than
// AI-credit spend on a new job.
func matchTextLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByUserOrIP("matchtext"), matchTextPerHour, time.Hour)
}

// matchTextRequest is a job posting scraped from an arbitrary page: its heading
// and visible text. No catalog job is required.
type matchTextRequest struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// textMatch derives skills from a posting's title+text with the same skilltag
// pass ingest uses for job.Skills, then scores their coverage by the profile.
// Deterministic, no LLM.
func textMatch(title, text string, profileSkills []string) jobmatch.JobMatch {
	skills := skilltag.Parse(title + "\n" + text)
	return jobmatch.Compute(skills, profileSkills)
}

// MatchText scores how well an arbitrary job posting (title + scraped text) is
// covered by the authenticated caller's profile skills — the same deterministic
// coverage as GET /jobs/:slug/match, but for a page that need not be in the
// freehire catalog. This is what lets the browser extension show a match on any
// job page. Cookie or API key; a caller without a profile is a 404.
func (h *matchHandlers) MatchText(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in matchTextRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	profile, err := h.userProfile.Get(c.Context(), userID)
	if err != nil {
		return profileError(err)
	}
	return c.JSON(fiber.Map{"data": textMatch(in.Title, in.Text, profile.Skills)})
}
