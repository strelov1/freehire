package handler

import (
	"context"
	"encoding/json"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
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
func (h *matchHandlers) GetJobMatch(ctx context.Context, params apigen.GetJobMatchParams) (*apigen.GetJobMatchOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err
	}
	if err := jobVisibleToCaller(job, userID, true); err != nil {
		return nil, err
	}
	profile, err := h.userProfile.Get(ctx, userID)
	if err != nil {
		return nil, profileError(err)
	}
	blockers := h.jobBlockers(ctx, userID, job, profile)
	if blockers == nil {
		blockers = []hardconstraint.Blocker{}
	}
	raw, err := json.Marshal(jobMatchResponse{
		JobMatch: jobmatch.Compute(job.Skills, profile.Skills),
		Blockers: blockers,
	})
	if err != nil {
		return nil, err
	}
	return &apigen.GetJobMatchOK{Data: raw}, nil
}
