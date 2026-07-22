package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/jobmatch"
)

// jobMatchResponse is the profile-match payload: the deterministic skill coverage
// plus the advisory hard-constraint blockers. Blockers is always present (empty
// when the caller has no structured résumé, so the bar degrades to coverage only).
type jobMatchResponse struct {
	jobmatch.JobMatch
	Blockers []hardconstraint.Blocker `json:"blockers"`
}

// JobMatch serves the per-job profile match: how well the open job's skills are
// covered by the authenticated caller's profile skills (exact/adjacent/missing +
// coverage percent), plus deterministic hard-constraint blockers (years, education,
// certifications, work authorization, location/work-mode) as advisory warnings —
// never hiding or downranking the job. Deterministic, no LLM. Cookie or API key; an
// unknown slug is a 404 (pgx.ErrNoRows via the central handler) and a caller without
// a profile is a 404 (profileError). The SPA only calls this once the viewer is
// signed in with a non-empty profile, so the not-found paths are defensive.
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
	blockers := a.jobBlockers(c.Context(), userID, job, profile)
	if blockers == nil {
		blockers = []hardconstraint.Blocker{}
	}
	return c.JSON(fiber.Map{"data": jobMatchResponse{
		JobMatch: jobmatch.Compute(job.Skills, profile.Skills),
		Blockers: blockers,
	}})
}
