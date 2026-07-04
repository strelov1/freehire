package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobmatch"
)

// JobMatch serves the per-job profile match: how well the open job's skills are
// covered by the authenticated caller's profile skills (exact/adjacent/missing +
// coverage percent). Deterministic, no LLM. Cookie or API key; an unknown slug is
// a 404 (pgx.ErrNoRows via the central handler) and a caller without a profile is
// a 404 (profileError). The SPA only calls this once the viewer is signed in with
// a non-empty profile, so the not-found paths are defensive.
func (a *API) JobMatch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := a.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	profile, err := a.userProfile.Get(c.Context(), userID)
	if err != nil {
		return profileError(err)
	}
	return c.JSON(fiber.Map{"data": jobmatch.Compute(job.Skills, profile.Skills)})
}
