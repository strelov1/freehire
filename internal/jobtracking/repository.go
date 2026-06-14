package jobtracking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/db"
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
	return id, err
}

// RecordView records (or refreshes) a user's view of a job.
func (r *QueriesRepository) RecordView(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
}

// MarkApplied marks a job as applied for a user.
func (r *QueriesRepository) MarkApplied(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.MarkJobApplied(ctx, db.MarkJobAppliedParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(row), nil
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

// TrackJob upserts stage and/or notes for the interaction. A nil pointer means
// "leave unchanged".
func (r *QueriesRepository) TrackJob(ctx context.Context, userID, jobID int64, stage, notes *string) (Interaction, error) {
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

// toInteraction converts a db.UserJob row to the domain Interaction type.
func toInteraction(r db.UserJob) Interaction {
	var viewedAt *time.Time
	if r.ViewedAt.Valid {
		t := r.ViewedAt.Time
		viewedAt = &t
	}

	var appliedAt *time.Time
	if r.AppliedAt.Valid {
		t := r.AppliedAt.Time
		appliedAt = &t
	}

	var savedAt *time.Time
	if r.SavedAt.Valid {
		t := r.SavedAt.Time
		savedAt = &t
	}

	var stage *string
	if r.Stage.Valid {
		s := r.Stage.String
		stage = &s
	}

	var notes *string
	if r.Notes.Valid {
		s := r.Notes.String
		notes = &s
	}

	return Interaction{
		JobID:     r.JobID,
		ViewedAt:  viewedAt,
		AppliedAt: appliedAt,
		SavedAt:   savedAt,
		Stage:     stage,
		Notes:     notes,
	}
}

// textFromPtr converts a *string to pgtype.Text. A nil pointer produces an
// invalid (NULL) Text so the SQL COALESCE leaves the column unchanged.
func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
