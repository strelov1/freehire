package submission

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/db"
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
func (r *QueriesRepository) Create(ctx context.Context, p db.CreateSubmissionParams) (db.JobSubmission, error) {
	sub, err := r.q.CreateSubmission(ctx, p)
	if isUniqueViolation(err) {
		return db.JobSubmission{}, ErrDuplicatePending
	}
	if err != nil {
		return db.JobSubmission{}, err
	}
	return sub, nil
}

// Get loads a submission by id, mapping a missing row to ErrSubmissionNotFound.
func (r *QueriesRepository) Get(ctx context.Context, id int64) (db.JobSubmission, error) {
	sub, err := r.q.GetSubmission(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.JobSubmission{}, ErrSubmissionNotFound
	}
	if err != nil {
		return db.JobSubmission{}, err
	}
	return sub, nil
}

// ListPending returns the pending review queue with submitter emails.
func (r *QueriesRepository) ListPending(ctx context.Context) ([]db.ListPendingSubmissionsRow, error) {
	return r.q.ListPendingSubmissions(ctx)
}

// ListByUser returns one user's submissions, each with the minted job's slug when approved.
func (r *QueriesRepository) ListByUser(ctx context.Context, userID int64) ([]db.ListSubmissionsByUserRow, error) {
	return r.q.ListSubmissionsByUser(ctx, userID)
}

// MarkApproved marks a pending submission approved. The query is scoped to status='pending',
// so a concurrent second decision affects no row — surfaced as ErrAlreadyDecided.
func (r *QueriesRepository) MarkApproved(ctx context.Context, p db.MarkSubmissionApprovedParams) (db.JobSubmission, error) {
	sub, err := r.q.MarkSubmissionApproved(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.JobSubmission{}, ErrAlreadyDecided
	}
	if err != nil {
		return db.JobSubmission{}, err
	}
	return sub, nil
}

// MarkRejected marks a pending submission rejected (see MarkApproved for the status scope).
func (r *QueriesRepository) MarkRejected(ctx context.Context, p db.MarkSubmissionRejectedParams) (db.JobSubmission, error) {
	sub, err := r.q.MarkSubmissionRejected(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.JobSubmission{}, ErrAlreadyDecided
	}
	if err != nil {
		return db.JobSubmission{}, err
	}
	return sub, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
