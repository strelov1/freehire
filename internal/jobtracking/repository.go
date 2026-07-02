package jobtracking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/userjob"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository interface.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository wraps q as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// JobIDBySlug returns the internal job id for the given public slug, or
// ErrJobNotFound when no job matches.
func (r *QueriesRepository) JobIDBySlug(ctx context.Context, slug string) (int64, error) {
	id, err := r.q.GetJobIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrJobNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// RecordView records (or refreshes) a user's view of a job.
func (r *QueriesRepository) RecordView(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	// The CTE that bumps view_count makes sqlc emit a bespoke row type with the
	// same shape as db.UserJob; convert so the interaction mapping stays shared.
	return toInteraction(db.UserJob(row)), nil
}

// MarkApplied marks a job as applied for a user.
func (r *QueriesRepository) MarkApplied(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	// See RecordView: the applied_count CTE yields a bespoke row of db.UserJob shape.
	return toInteraction(db.UserJob(row)), nil
}

// SaveJob saves (bookmarks) a job for a user.
func (r *QueriesRepository) SaveJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.SaveJob(ctx, db.SaveJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// UnsaveJob clears the saved mark. Returns ErrNoInteraction when no row exists.
func (r *QueriesRepository) UnsaveJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.UnsaveJob(ctx, db.UnsaveJobParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, ErrNoInteraction
	}
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// DismissJob marks a job dismissed (swiped away) for a user.
func (r *QueriesRepository) DismissJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.DismissJob(ctx, db.DismissJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// UndismissJob clears the dismissed mark. Returns ErrNoInteraction when no row exists.
func (r *QueriesRepository) UndismissJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.UndismissJob(ctx, db.UndismissJobParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, ErrNoInteraction
	}
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// TrackJob upserts stage and/or notes for the interaction. A nil pointer means
// "leave unchanged".
func (r *QueriesRepository) TrackJob(
	ctx context.Context,
	userID, jobID int64,
	stage, notes *string,
) (Interaction, error) {
	row, err := r.q.TrackJob(ctx, db.TrackJobParams{
		UserID: userID,
		JobID:  jobID,
		Stage:  textFromPtr(stage),
		Notes:  textFromPtr(notes),
	})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// ClearJobProgress drops stage and applied_at for a user's interaction with a job.
func (r *QueriesRepository) ClearJobProgress(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.ClearJobProgress(ctx, db.ClearJobProgressParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// UntrackJob removes a job from the board by clearing all pipeline marks except viewed_at.
func (r *QueriesRepository) UntrackJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.UntrackJob(ctx, db.UntrackJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// ListInteractions returns the caller's interactions joined with the jobs in the
// canonical jobview shape, narrowed by the already-validated filter.
func (r *QueriesRepository) ListInteractions(
	ctx context.Context,
	userID int64,
	filter Filter,
	limit, offset int32,
) ([]TrackedJob, error) {
	rows, err := r.q.ListUserJobs(ctx, db.ListUserJobsParams{
		UserID: userID,
		Filter: string(filter),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]TrackedJob, 0, len(rows))
	for _, row := range rows {
		view, err := jobview.FromRow(row.Job)
		if err != nil {
			return nil, err
		}
		items = append(items, TrackedJob{
			Job: view,
			Interaction: Interaction{
				JobID:     row.Job.ID,
				ViewedAt:  timePtr(row.ViewedAt),
				SavedAt:   timePtr(row.SavedAt),
				AppliedAt: timePtr(row.AppliedAt),
				Stage:     textPtr(row.Stage),
				Notes:     textPtr(row.Notes),
			},
		})
	}
	return items, nil
}

// CountInteractions returns the per-filter counts for the caller in one pass.
func (r *QueriesRepository) CountInteractions(ctx context.Context, userID int64) (Counts, error) {
	row, err := r.q.CountUserJobs(ctx, userID)
	if err != nil {
		return Counts{}, err
	}
	return Counts{
		All:     row.All,
		Viewed:  row.Viewed,
		Saved:   row.Saved,
		Applied: row.Applied,
		Board:   row.Board,
	}, nil
}

// ViewedSlugs returns every public job slug the caller has interacted with.
func (r *QueriesRepository) ViewedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return r.q.ListViewedJobSlugs(ctx, userID)
}

// ExcludedJobIDs returns up to limit job ids the caller has saved or dismissed.
func (r *QueriesRepository) ExcludedJobIDs(ctx context.Context, userID int64, limit int32) ([]int64, error) {
	return r.q.ExcludedJobIDs(ctx, db.ExcludedJobIDsParams{UserID: userID, Limit: limit})
}

// PipelineCounts returns the caller's per-stage application counts. A NULL stage
// (an applied row with no explicit stage) becomes an empty Stage, which
// userjob.Aggregate folds into the no_answer bucket.
func (r *QueriesRepository) PipelineCounts(ctx context.Context, userID int64) ([]userjob.StageCount, error) {
	rows, err := r.q.CountMyJobsByStage(ctx, userID)
	if err != nil {
		return nil, err
	}
	counts := make([]userjob.StageCount, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, userjob.StageCount{Stage: row.Stage.String, Count: row.Count})
	}
	return counts, nil
}

// toInteraction converts a db.UserJob row to the domain Interaction type.
func toInteraction(r db.UserJob) Interaction {
	return Interaction{
		JobID:       r.JobID,
		ViewedAt:    timePtr(r.ViewedAt),
		AppliedAt:   timePtr(r.AppliedAt),
		SavedAt:     timePtr(r.SavedAt),
		DismissedAt: timePtr(r.DismissedAt),
		Stage:       textPtr(r.Stage),
		Notes:       textPtr(r.Notes),
	}
}

// timePtr converts a pgtype.Timestamptz to *time.Time (nil when NULL).
func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// textPtr converts a pgtype.Text to *string (nil when NULL).
func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// textFromPtr converts a *string to pgtype.Text. A nil pointer produces an
// invalid (NULL) Text so the SQL COALESCE leaves the column unchanged.
func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
