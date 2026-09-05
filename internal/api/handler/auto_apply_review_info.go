package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/platform/db"
)

// autoApplyReviewInfoForJob reads the caller's live auto-apply attempt for job, if any, and
// assembles the candidate-facing view the tracker drawer shows (openspec/changes/
// auto-apply-review-tracking). A missing entry is not an error and returns (nil, nil) — most
// tracked jobs have never had an auto-apply attempt.
func autoApplyReviewInfoForJob(ctx context.Context, queries *db.Queries, userID, jobID int64) (*autoapply.AutoApplyReviewInfo, error) {
	row, err := queries.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	attempt := autoapply.ResolvedAttempt{
		QueueID:        row.ID,
		HasTailoredCV:  row.TailoredCvID != nil,
		ReviewDecision: pgStr(row.ReviewDecision),
		Blocked:        row.BlockedAt.Valid,
		Failed:         row.FailedAt.Valid,
	}
	if len(row.Unmapped) > 0 {
		if err := json.Unmarshal(row.Unmapped, &attempt.Unmapped); err != nil {
			return nil, fmt.Errorf("decode unmapped fields for queue entry %d: %w", row.ID, err)
		}
	}
	if len(row.ResolvedPreview) > 0 {
		var preview autoapply.ResolvedPreview
		if err := json.Unmarshal(row.ResolvedPreview, &preview); err != nil {
			return nil, fmt.Errorf("decode resolved preview for queue entry %d: %w", row.ID, err)
		}
		attempt.ResolvedPreview = &preview
	}
	return autoapply.AssembleReviewInfo(true, attempt), nil
}
