package handler

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/ghost"
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
func ghostEvidenceFor(ctx context.Context, q *db.Queries, jobIDs []int64) map[int64]ghost.Evidence {
	if q == nil || len(jobIDs) == 0 {
		return nil
	}
	appRows, err := q.ListGhostApplicationEvidence(ctx, jobIDs)
	if err != nil {
		return nil
	}
	reportRows, err := q.ListGhostReportEvidence(ctx, jobIDs)
	if err != nil {
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
