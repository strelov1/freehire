package handler

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/platform/db"
)

// ghostEvidenceFor gathers the outcome evidence for a page of jobs in two queries. The
// map is sparse: only jobs anyone applied to or reported appear.
//
// Shared by the job read paths and the search path, so the closed-job check lives once.
//
// Best-effort: a lookup failure yields no evidence and downgrades the signal rather than
// failing the read. It is logged, though: losing these reads makes LevelLikely unreachable
// and the badge would silently stop appearing.
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

// logGhostLookup records a failed best-effort ghost-signal read, saying whether the page
// went away or the database did.
//
// It classifies on the context, not the error (as searchdrain and embed do): a reader who
// navigates away cancels every in-flight query, and counting those as faults would bury
// the real ones.
func logGhostLookup(ctx context.Context, stage string, jobs int, err error) {
	if cause := ctx.Err(); cause != nil {
		log.Printf("ghost: %s for %d jobs abandoned (%v) — signal omitted", stage, jobs, cause)
		return
	}
	log.Printf("ghost: %s for %d jobs failed, serving the page without the outcome tier: %v", stage, jobs, err)
}
