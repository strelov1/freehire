package handler

import (
	"context"
	"encoding/json"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

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
func (h *matchHandlers) MatchText(ctx context.Context, req *apigen.FitMatchTextRequest) (*apigen.MatchTextOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.userProfile.Get(ctx, userID)
	if err != nil {
		return nil, profileError(err)
	}
	raw, err := json.Marshal(textMatch(req.Title.Value, req.Text.Value, profile.Skills))
	if err != nil {
		return nil, err
	}
	return &apigen.MatchTextOK{Data: raw}, nil
}
