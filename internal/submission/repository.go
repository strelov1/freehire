package submission

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository. Each method maps the relevant
// Postgres condition onto a package sentinel: a unique violation on create → duplicate
// pending, no row on get → not found, no row on a status-scoped mark → already decided.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Create inserts a pending submission. The partial unique index on lower(url) WHERE
// status='pending' rejects a second pending submission of the same URL; that surfaces as
// ErrDuplicatePending.
func (r *QueriesRepository) Create(ctx context.Context, submittedBy int64, in moderation.CreateInput) (Submission, error) {
	sub, err := r.q.CreateSubmission(ctx, db.CreateSubmissionParams{
		SubmittedBy: submittedBy,
		URL:         in.URL,
		Source:      in.Source,
		Title:       in.Title,
		Company:     in.Company,
		Location:    in.Location,
		Remote:      in.Remote,
		Description: in.Description,
		PostedAt:    pgconv.Timestamptz(in.PostedAt),
	})
	if pgerr.IsUniqueViolation(err) {
		return Submission{}, ErrDuplicatePending
	}
	if err != nil {
		return Submission{}, err
	}
	return fromRow(sub), nil
}

// Get loads a submission by id, mapping a missing row to ErrSubmissionNotFound.
func (r *QueriesRepository) Get(ctx context.Context, id int64) (Submission, error) {
	sub, err := r.q.GetSubmission(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrSubmissionNotFound
	}
	if err != nil {
		return Submission{}, err
	}
	return fromRow(sub), nil
}

// ListPending returns the pending review queue with submitter emails.
func (r *QueriesRepository) ListPending(ctx context.Context) ([]PendingSubmission, error) {
	rows, err := r.q.ListPendingSubmissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingSubmission, len(rows))
	for i, row := range rows {
		out[i] = fromPendingRow(row)
	}
	return out, nil
}

// ListByUser returns one user's submissions, each with the minted job's slug when approved.
func (r *QueriesRepository) ListByUser(ctx context.Context, userID int64) ([]UserSubmission, error) {
	rows, err := r.q.ListSubmissionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]UserSubmission, len(rows))
	for i, row := range rows {
		out[i] = fromUserRow(row)
	}
	return out, nil
}

// MarkApproved marks a pending submission approved. The query is scoped to status='pending',
// so a concurrent second decision affects no row — surfaced as ErrAlreadyDecided.
func (r *QueriesRepository) MarkApproved(ctx context.Context, id, reviewerID, jobID int64) (Submission, error) {
	sub, err := r.q.MarkSubmissionApproved(ctx, db.MarkSubmissionApprovedParams{
		ID:         id,
		ReviewedBy: reviewerID,
		JobID:      jobID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrAlreadyDecided
	}
	if err != nil {
		return Submission{}, err
	}
	return fromRow(sub), nil
}

// MarkRejected marks a pending submission rejected (see MarkApproved for the status scope).
func (r *QueriesRepository) MarkRejected(ctx context.Context, id, reviewerID int64, reason string) (Submission, error) {
	sub, err := r.q.MarkSubmissionRejected(ctx, db.MarkSubmissionRejectedParams{
		ID:           id,
		ReviewedBy:   reviewerID,
		ReviewReason: reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrAlreadyDecided
	}
	if err != nil {
		return Submission{}, err
	}
	return fromRow(sub), nil
}

// fromRow maps the generated db row to the package domain type.
func fromRow(row db.JobSubmission) Submission {
	return Submission{
		ID:           row.ID,
		SubmittedBy:  row.SubmittedBy,
		URL:          row.URL,
		Source:       row.Source,
		Title:        row.Title,
		Company:      row.Company,
		Location:     row.Location,
		Remote:       row.Remote,
		Description:  row.Description,
		PostedAt:     pgconv.TimePtr(row.PostedAt),
		Status:       row.Status,
		ReviewReason: row.ReviewReason,
		ReviewedAt:   pgconv.TimePtr(row.ReviewedAt),
		CreatedAt:    pgconv.TimePtr(row.CreatedAt),
	}
}

// fromPendingRow maps a moderator-queue row to PendingSubmission, adding the submitter email.
func fromPendingRow(row db.ListPendingSubmissionsRow) PendingSubmission {
	return PendingSubmission{
		Submission: Submission{
			ID:           row.ID,
			SubmittedBy:  row.SubmittedBy,
			URL:          row.URL,
			Source:       row.Source,
			Title:        row.Title,
			Company:      row.Company,
			Location:     row.Location,
			Remote:       row.Remote,
			Description:  row.Description,
			PostedAt:     pgconv.TimePtr(row.PostedAt),
			Status:       row.Status,
			ReviewReason: row.ReviewReason,
			ReviewedAt:   pgconv.TimePtr(row.ReviewedAt),
			CreatedAt:    pgconv.TimePtr(row.CreatedAt),
		},
		SubmitterEmail: row.SubmitterEmail,
	}
}

// fromUserRow maps a "my submissions" row to UserSubmission, adding the minted job's slug
// (empty when the submission has not been approved into a live vacancy).
func fromUserRow(row db.ListSubmissionsByUserRow) UserSubmission {
	return UserSubmission{
		Submission: Submission{
			ID:           row.ID,
			SubmittedBy:  row.SubmittedBy,
			URL:          row.URL,
			Source:       row.Source,
			Title:        row.Title,
			Company:      row.Company,
			Location:     row.Location,
			Remote:       row.Remote,
			Description:  row.Description,
			PostedAt:     pgconv.TimePtr(row.PostedAt),
			Status:       row.Status,
			ReviewReason: row.ReviewReason,
			ReviewedAt:   pgconv.TimePtr(row.ReviewedAt),
			CreatedAt:    pgconv.TimePtr(row.CreatedAt),
		},
		JobSlug: row.JobSlug.String,
	}
}
