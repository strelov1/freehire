package handler

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/platform/db"
)

// ghostEvidenceFor gathers the outcome evidence for a page of jobs in two queries
// rather than two per card. The returned map is sparse — only jobs anyone applied to
// or reported appear in it — so a page with no evidence costs two empty result sets.
//
// Shared by the job read paths and the search path, which need the same evidence from
// different starting shapes. Two copies of this drifted apart the moment one of them
// gained the closed-job check.
//
// Best-effort: a lookup failure yields no evidence, which downgrades or removes the
// signal rather than failing the read. That is the honest direction — a missing lookup
// is a gap in what we know, and the signal's whole discipline is to say nothing where
// it has not observed.
//
// Best-effort is not silent, though. Losing these two reads takes the whole outcome
// tier out of the verdict — LevelLikely needs evidence and becomes unreachable — and
// there is no QueryTracer in internal/platform/db, so a swallowed error left no trace
// anywhere: the badge simply stopped appearing and every page still answered 200.
func ghostEvidenceFor(ctx context.Context, q *db.Queries, jobIDs []int64) map[int64]ghost.Evidence {
	if q == nil || len(jobIDs) == 0 {
		return nil
	}
	appRows, err := q.ListGhostApplicationEvidence(ctx, jobIDs)
	if err != nil {
		logGhostLookup(ctx, "application evidence", len(jobIDs), err)
		return nil
	}
	reportRows, err := q.ListGhostReportEvidence(ctx, jobIDs)
	if err != nil {
		logGhostLookup(ctx, "report evidence", len(jobIDs), err)
		return nil
	}

	apps := make([]ghost.Application, len(appRows))
	for i, r := range appRows {
		apps[i] = ghost.Application{
			JobID:                r.JobID,
			UserID:               r.UserID,
			Stage:                r.Stage,
			LastActivityAt:       r.LastActivityAt.Time,
			HasPendingSuggestion: r.HasPendingSuggestion,
		}
	}
	reports := make([]ghost.Report, len(reportRows))
	for i, r := range reportRows {
		reports[i] = ghost.Report{JobID: r.JobID, UserID: r.UserID, AppliedOn: r.AppliedOn.Time}
	}
	return ghost.Aggregate(time.Now(), apps, reports)
}

// logGhostLookup records a best-effort ghost-signal read that failed, saying whether
// the page simply went away or the database did.
//
// It classifies on the CONTEXT, not on the error — the idiom searchdrain and embed
// settled on. A reader who navigates away cancels the request, every in-flight query
// fails with it, and counting those as faults would bury the one line that means
// something under the commonest event on the site. Reading ctx.Err() answers that
// without needing the driver to cooperate with errors.Is.
func logGhostLookup(ctx context.Context, stage string, jobs int, err error) {
	if cause := ctx.Err(); cause != nil {
		log.Printf("ghost: %s for %d jobs abandoned (%v) — signal omitted", stage, jobs, cause)
		return
	}
	log.Printf("ghost: %s for %d jobs failed, serving the page without the outcome tier: %v", stage, jobs, err)
}
