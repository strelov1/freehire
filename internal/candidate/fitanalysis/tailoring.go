package fitanalysis

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// ErrNoAnalysis reports that the candidate has not run the fit analysis for this job yet, or
// that what was stored is unreadable — which reads the same to every caller, since both mean
// "there is nothing to ground on, offer them a run".
//
// It is a state, not a failure, and the two readers treat it differently on purpose: the
// tailoring surfaces cannot proceed without an analysis and refuse (the port renders 409),
// while the CV-vs-job score carries on and says so in the result. That is why the decision is
// a sentinel here rather than a status invented at either call site.
var ErrNoAnalysis = errors.New("fitanalysis: no analysis has been run for this job")

// Required returns the cached analysis, or ErrNoAnalysis when the candidate has not run one.
// For the readers that cannot do their job without it.
//
// A service with no store — a partially wired harness — reads as "no analysis" rather than
// panicking, the same way Balance answers nil. A tool runs inside an SSE writer's goroutine
// where a panic reaches no recover, so a missing collaborator has to degrade, not explode.
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
// carry on without it.
//
// A failed read degrades to "none" the same way an absent one does — the caller's other
// signals are worth more than a refusal — but is logged, because a broken database must not
// look to us like a candidate who never ran their analysis.
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
// on — MissingHave (reframe existing evidence) vs MissingGap (ask the candidate first).
//
// One projection serves both readers — the HTTP endpoint and the agent's tool — so neither
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
	// Requirements is what the POSTING asks for, in its own words — a different thing
	// from the MissingHave/MissingGap split above, which is what an ANALYSIS decided the
	// CV lacks. The agent needs both: the split says what to act on, this says what the
	// employer actually wrote, including the requirements already covered, which are
	// exactly the ones worth keeping prominent rather than editing away.
	//
	// It matters more than it looks. The description below is clipped, and a posting's
	// requirement list is nearly always at its END — so on a long posting the list was
	// the first thing to fall off, and the agent could not see the requirements at all.
	//
	// Supplied by the caller rather than read from `job` here: the two producers of this
	// list (the model's enrichment and the markup parser's derived column) are reconciled
	// in exactly one place, `jobview.FromDomain`, and that lives in a block this one sits
	// below and may not import. Merging it a second time here is the drift that rule
	// exists to prevent.
	//
	// It is not free. Bounded at write time by enrich.BoundRequirements — 30 entries of at
	// most 200 runes — so the worst case is about 6000 runes, the same order as the clipped
	// description beside it, on every tailoring turn. Real postings are far under that, and
	// a requirement list is the densest thing in a posting per token spent, which is what
	// makes the trade worth it.
	//
	// Absent, not empty, when there is none: `[]` would read to the agent as an employer
	// who asked for nothing, which no posting means.
	Requirements []enrich.Requirement `json:"requirements,omitempty"`
	Description  string               `json:"description"`
}

// descriptionLimit bounds the posting in a tailoring context. The posting is the largest thing
// in the turn and the least trusted, so it is clipped as well as stripped of markup.
const descriptionLimit = 6000

// TailoringContext assembles what a tailoring reader needs: the cached analysis, projected over
// the vacancy it is about. The job is supplied rather than read here — every caller has already
// loaded and authorized it — so this package needs no job port of its own.
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
			// The posting reaches the model as words, bounded, the same way get_job already
			// serves it. Sending markup spends the context window on tags and widens what
			// there is to misread.
			Description: clipRunes(htmltext.ToMarkdown(job.Description), descriptionLimit),
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

// clipRunes truncates on a rune boundary so a clip never splits a multi-byte character.
func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}
