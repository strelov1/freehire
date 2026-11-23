package nudge

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/db"
)

// matchAutoApply records the KindAutoApply* nudges: the three terminal outcomes of an
// unattended attempt, and the reminder that a tailored entry waits for review.
func (r *Runner) matchAutoApply(ctx context.Context, stats *Stats) error {
	submitted, err := r.store.ListAutoApplySubmittedCandidates(ctx, r.cfg.AutoApplyOutcomeWindowDays)
	if err != nil {
		return fmt.Errorf("list auto-apply-submitted candidates: %w", err)
	}
	for _, s := range submitted {
		if !s.JobID.Valid || !s.OccurredAt.Valid {
			continue // defensive: the query's WHERE clause already guarantees both are set
		}
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: s.UserID, JobID: s.JobID.Int64, Kind: KindAutoApplySubmitted,
			EpisodeKey: s.OccurredAt,
		})
		if err != nil {
			return fmt.Errorf("record auto-apply-submitted nudge: %w", err)
		}
		stats.Matched += int(affected)
	}

	blocked, err := r.store.ListAutoApplyBlockedCandidates(ctx, r.cfg.AutoApplyOutcomeWindowDays)
	if err != nil {
		return fmt.Errorf("list auto-apply-blocked candidates: %w", err)
	}
	for _, b := range blocked {
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: b.UserID, JobID: b.JobID, Kind: KindAutoApplyBlocked,
			EpisodeKey: b.BlockedAt,
		})
		if err != nil {
			return fmt.Errorf("record auto-apply-blocked nudge: %w", err)
		}
		stats.Matched += int(affected)
	}

	failed, err := r.store.ListAutoApplyFailedCandidates(ctx, r.cfg.AutoApplyOutcomeWindowDays)
	if err != nil {
		return fmt.Errorf("list auto-apply-failed candidates: %w", err)
	}
	for _, f := range failed {
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: f.UserID, JobID: f.JobID, Kind: KindAutoApplyFailed,
			EpisodeKey: f.FailedAt,
		})
		if err != nil {
			return fmt.Errorf("record auto-apply-failed nudge: %w", err)
		}
		stats.Matched += int(affected)
	}

	ready, err := r.store.ListAutoApplyReviewReadyCandidates(ctx, r.cfg.AutoApplyReviewWindowDays)
	if err != nil {
		return fmt.Errorf("list auto-apply-review-ready candidates: %w", err)
	}
	for _, q := range ready {
		affected, err := r.store.RecordNudge(ctx, db.RecordNudgeParams{
			UserID: q.UserID, JobID: q.JobID, Kind: KindAutoApplyReviewReady,
			EpisodeKey: q.CreatedAt,
		})
		if err != nil {
			return fmt.Errorf("record auto-apply-review-ready nudge: %w", err)
		}
		stats.Matched += int(affected)
	}
	return nil
}
