package report

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/db"
)

// Compile-time proof that QueriesRepository satisfies both the persistence contract and the
// job-close seam.
var (
	_ Repository = (*QueriesRepository)(nil)
	_ JobCloser  = (*QueriesRepository)(nil)
)

// QueriesRepository adapts *db.Queries to the Repository and JobCloser. Each method maps the
// relevant Postgres condition onto a package sentinel: a unique violation on create →
// duplicate open, no row on get → not found, no row on a status-scoped mark → already
// decided.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Create inserts a pending report. The partial unique index on (reported_by, job_id) WHERE
// status='pending' rejects a second open report of the same job by the same user; that
// surfaces as ErrDuplicateOpen.
func (r *QueriesRepository) Create(ctx context.Context, p db.CreateReportParams) (db.JobReport, error) {
	rep, err := r.q.CreateReport(ctx, p)
	if isUniqueViolation(err) {
		return db.JobReport{}, ErrDuplicateOpen
	}
	if err != nil {
		return db.JobReport{}, err
	}
	return rep, nil
}

// Get loads a report by id, mapping a missing row to ErrReportNotFound.
func (r *QueriesRepository) Get(ctx context.Context, id int64) (db.JobReport, error) {
	rep, err := r.q.GetReport(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.JobReport{}, ErrReportNotFound
	}
	if err != nil {
		return db.JobReport{}, err
	}
	return rep, nil
}

// ListPending returns the pending review queue with reporter email and job slug/title.
func (r *QueriesRepository) ListPending(ctx context.Context) ([]db.ListPendingReportsRow, error) {
	return r.q.ListPendingReports(ctx)
}

// MarkResolved marks a pending report resolved. The query is scoped to status='pending', so
// a concurrent second decision affects no row — surfaced as ErrAlreadyDecided.
func (r *QueriesRepository) MarkResolved(ctx context.Context, p db.MarkReportResolvedParams) (db.JobReport, error) {
	rep, err := r.q.MarkReportResolved(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.JobReport{}, ErrAlreadyDecided
	}
	if err != nil {
		return db.JobReport{}, err
	}
	return rep, nil
}

// MarkDismissed marks a pending report dismissed (see MarkResolved for the status scope).
func (r *QueriesRepository) MarkDismissed(ctx context.Context, p db.MarkReportDismissedParams) (db.JobReport, error) {
	rep, err := r.q.MarkReportDismissed(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.JobReport{}, ErrAlreadyDecided
	}
	if err != nil {
		return db.JobReport{}, err
	}
	return rep, nil
}

// Close soft-closes one job (CloseJobByID is idempotent — closing an already-closed job
// affects no row and is not an error).
func (r *QueriesRepository) Close(ctx context.Context, jobID int64) error {
	_, err := r.q.CloseJobByID(ctx, jobID)
	return err
}

// isUniqueViolation reports whether err is a Postgres unique-constraint violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
