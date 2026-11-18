package fitanalysis

import (
	"context"
	"errors"
	"log"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// ErrNoAnalysis reports that the candidate has not run the fit analysis for this job yet, or
// that what was stored is unreadable. It is a state, not a failure: the tailoring surfaces
// refuse without an analysis (the port renders 409), while the CV-vs-job score carries on and
// says so.
var ErrNoAnalysis = errors.New("fitanalysis: no analysis has been run for this job")

// Required returns the cached analysis, or ErrNoAnalysis when the candidate has not run one.
// For the readers that cannot do their job without it.
//
// A service with no store reads as "no analysis" rather than panicking: a tool runs in an SSE
// writer's goroutine where a panic reaches no recover.
func (s *Service) Required(ctx context.Context, userID, jobID int64) (*matchanalysis.Analysis, error) {
	if s == nil || s.store == nil {
		return nil, ErrNoAnalysis
	}
	analysis, _, err := s.Cached(ctx, userID, jobID)
	if err != nil {
		return nil, err
	}
	if analysis == nil {
		return nil, ErrNoAnalysis
	}
	return analysis, nil
}

// Optional returns the cached analysis and whether there is one, for the readers that can
// carry on without it. A failed read degrades to "none" but is logged, so a broken database
// does not look like a candidate who never ran their analysis.
func (s *Service) Optional(ctx context.Context, userID, jobID int64) (*matchanalysis.Analysis, bool) {
	if s == nil || s.store == nil {
		return nil, false
	}
	analysis, _, err := s.Cached(ctx, userID, jobID)
	if err != nil {
		log.Printf("fitanalysis: cached analysis for user %d job %d: %v", userID, jobID, err)
		return nil, false
	}
	return analysis, analysis != nil
}

// TailoringContext is the reasoning context a tailoring reader gets: the vacancy, the verdict
// and recommendation, per-dimension comments, and the requirement split the honest wall turns
// on: MissingHave (reframe existing evidence) vs MissingGap (ask the candidate first). One
// projection serves the HTTP endpoint and the agent's tool so they cannot drift.
// can drift into showing a different shape of the same analysis.
type TailoringContext struct {
	Job            TailoringJob                `json:"job"`
	Verdict        string                      `json:"verdict"`
	OverallScore   int                         `json:"overall_score"`
	Recommendation string                      `json:"recommendation"`
	Dimensions     []matchanalysis.Dimension   `json:"dimensions"`
	MissingHave    []matchanalysis.Requirement `json:"missing_have"`
	MissingGap     []matchanalysis.Requirement `json:"missing_gap"`
	Strengths      []string                    `json:"strengths"`
	Gaps           []string                    `json:"gaps"`
}

// TailoringJob is the vacancy as a tailoring reader sees it.
type TailoringJob struct {
	Title   string `json:"title"`
	Company string `json:"company"`
	Slug    string `json:"slug"`
	// Requirements is what the POSTING asks for, in its own words, as opposed to the
	// MissingHave/MissingGap split, which is what the ANALYSIS decided the CV lacks. It includes
	// requirements already covered, which are the ones worth keeping prominent.
	//
	// It matters because the description is clipped and a requirement list is nearly always at
	// the END of a posting.
	//
	// Supplied by the caller: the model's enrichment and the markup parser's column are
	// reconciled once in `jobview.FromDomain`, in a block this one may not import.
	//
	// Bounded at write time by enrich.BoundRequirements (30 entries of at most 200 runes).
	//
	// Absent, not empty, when there is none: `[]` would read as an employer who asked for
	// nothing.
	Requirements []enrich.Requirement `json:"requirements,omitempty"`
	Description  string               `json:"description"`
}

// descriptionLimit bounds the posting in a tailoring context. The posting is the largest and
// least trusted part of the turn, so it is clipped as well as stripped of markup.
const descriptionLimit = 6000

// TailoringContext assembles what a tailoring reader needs: the cached analysis, projected over
// the vacancy. The caller supplies the job, already loaded and authorized.
func (s *Service) TailoringContext(ctx context.Context, userID int64, job db.Job, requirements []enrich.Requirement) (TailoringContext, error) {
	analysis, err := s.Required(ctx, userID, job.ID)
	if err != nil {
		return TailoringContext{}, err
	}
	return ProjectTailoring(analysis, job, requirements), nil
}

// ProjectTailoring splits an analysis's requirements into the reframe-able and the genuine
// gaps and pairs them with the vacancy. Pure, so the split is unit-testable without a store.
func ProjectTailoring(a *matchanalysis.Analysis, job db.Job, requirements []enrich.Requirement) TailoringContext {
	var have, gap []matchanalysis.Requirement
	for _, r := range a.RequirementMatch {
		switch r.Status {
		case matchanalysis.StatusMissingHave:
			have = append(have, r)
		case matchanalysis.StatusMissingGap:
			gap = append(gap, r)
		}
	}
	return TailoringContext{
		Job: TailoringJob{
			Title:        job.Title,
			Company:      job.Company,
			Slug:         job.PublicSlug,
			Requirements: requirements,
			// The posting reaches the model as bounded words, as get_job serves it; markup wastes
			// context on tags.
			Description: llm.ClipRunesEllipsis(htmltext.ToMarkdown(job.Description), descriptionLimit),
		},
		Verdict:        a.Verdict,
		OverallScore:   a.OverallScore,
		Recommendation: a.Recommendation,
		Dimensions:     a.Dimensions,
		MissingHave:    have,
		MissingGap:     gap,
		Strengths:      a.Strengths,
		Gaps:           a.Gaps,
	}
}
