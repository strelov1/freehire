package report

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
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
func (r *QueriesRepository) Create(ctx context.Context, reportedBy, jobID int64, reason, details, contactTelegram string) (Report, error) {
	rep, err := r.q.CreateReport(ctx, db.CreateReportParams{
		ReportedBy:      reportedBy,
		JobID:           jobID,
		Reason:          reason,
		Details:         details,
		ContactTelegram: contactTelegram,
	})
	if pgerr.IsUniqueViolation(err) {
		return Report{}, ErrDuplicateOpen
	}
	if err != nil {
		return Report{}, err
	}
	return fromRow(rep), nil
}

// Get loads a report by id, mapping a missing row to ErrReportNotFound.
func (r *QueriesRepository) Get(ctx context.Context, id int64) (Report, error) {
	rep, err := r.q.GetReport(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrReportNotFound
	}
	if err != nil {
		return Report{}, err
	}
	return fromRow(rep), nil
}

// ListPending returns the pending review queue with reporter email and job slug/title.
func (r *QueriesRepository) ListPending(ctx context.Context) ([]PendingReport, error) {
	rows, err := r.q.ListPendingReports(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingReport, len(rows))
	for i, row := range rows {
		out[i] = fromPendingRow(row)
	}
	return out, nil
}

// MarkResolved marks a pending report resolved. The query is scoped to status='pending', so
// a concurrent second decision affects no row — surfaced as ErrAlreadyDecided.
func (r *QueriesRepository) MarkResolved(ctx context.Context, id, reviewedBy int64) (Report, error) {
	rep, err := r.q.MarkReportResolved(ctx, db.MarkReportResolvedParams{ID: id, ReviewedBy: reviewedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrAlreadyDecided
	}
	if err != nil {
		return Report{}, err
	}
	return fromRow(rep), nil
}

// MarkDismissed marks a pending report dismissed (see MarkResolved for the status scope).
func (r *QueriesRepository) MarkDismissed(ctx context.Context, id, reviewedBy int64, reviewReason string) (Report, error) {
	rep, err := r.q.MarkReportDismissed(ctx, db.MarkReportDismissedParams{
		ID:           id,
		ReviewedBy:   reviewedBy,
		ReviewReason: reviewReason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrAlreadyDecided
	}
	if err != nil {
		return Report{}, err
	}
	return fromRow(rep), nil
}

// Close soft-closes one job (CloseJobByID is idempotent — closing an already-closed job
// affects no row and is not an error).
func (r *QueriesRepository) Close(ctx context.Context, jobID int64) error {
	_, err := r.q.CloseJobByID(ctx, jobID)
	return err
}

// fromRow maps the generated db row to the package domain type, dropping the internal
// ownership columns the use case does not need.
func fromRow(row db.JobReport) Report {
	return Report{
		ID:              row.ID,
		JobID:           row.JobID,
		Reason:          row.Reason,
		Details:         row.Details,
		ContactTelegram: row.ContactTelegram,
		Status:          row.Status,
		ReviewReason:    row.ReviewReason,
		ReviewedAt:      pgconv.TimePtr(row.ReviewedAt),
		CreatedAt:       pgconv.TimePtr(row.CreatedAt),
	}
}

// fromPendingRow maps a moderator-queue row to PendingReport, adding the joined reporter and
// job columns.
func fromPendingRow(row db.ListPendingReportsRow) PendingReport {
	return PendingReport{
		Report: Report{
			ID:              row.ID,
			JobID:           row.JobID,
			Reason:          row.Reason,
			Details:         row.Details,
			ContactTelegram: row.ContactTelegram,
			Status:          row.Status,
			ReviewReason:    row.ReviewReason,
			ReviewedAt:      pgconv.TimePtr(row.ReviewedAt),
			CreatedAt:       pgconv.TimePtr(row.CreatedAt),
		},
		ReporterEmail: row.ReporterEmail,
		JobSlug:       row.JobSlug,
		JobTitle:      row.JobTitle,
	}
}
