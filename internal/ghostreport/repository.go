package ghostreport

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// Compile-time proof that QueriesRepository satisfies the persistence contract.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Create files a claim, reviving one the same person previously retracted.
//
// The insert is gated in SQL — an unverified address or a closed job selects nothing —
// so every refusal arrives here identically, as no row. Which gate refused is then asked
// for once, on the failure path only. Guessing instead would mean either a check the
// service could forget or three round trips on the happy path.
func (r *QueriesRepository) Create(ctx context.Context, userID, jobID int64, appliedOn time.Time) (Report, error) {
	row, err := r.q.CreateGhostReport(ctx, db.CreateGhostReportParams{
		UserID:    userID,
		JobID:     jobID,
		AppliedOn: pgtype.Date{Time: appliedOn, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, r.refusal(ctx, userID, jobID)
	}
	if err != nil {
		return Report{}, err
	}
	return toReport(row), nil
}

// refusal asks which gate rejected the insert and maps it to a sentinel. The order is
// deliberate: an account that never proved its address should be told to confirm it,
// not that somebody already reported the job.
func (r *QueriesRepository) refusal(ctx context.Context, userID, jobID int64) error {
	gate, err := r.q.GhostReportRefusalReason(ctx, db.GhostReportRefusalReasonParams{
		UserID: userID,
		JobID:  jobID,
	})
	if err != nil {
		return err
	}
	switch {
	case !gate.EmailVerified:
		return ErrUnverified
	case !gate.JobOpen:
		return ErrJobClosed
	case gate.AlreadyReported:
		return ErrDuplicate
	default:
		// Every gate reports itself satisfied yet nothing was written. Rather than
		// invent a verdict, surface it: a silent success here would drop a person's
		// report while telling them it landed.
		return errors.New("ghostreport: the claim was refused for no recorded reason")
	}
}

// Retract withdraws a live claim; a claim that is absent or already withdrawn is
// ErrNotFound.
func (r *QueriesRepository) Retract(ctx context.Context, userID, jobID int64) error {
	_, err := r.q.RetractGhostReport(ctx, db.RetractGhostReportParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// CountFiledSince backs the daily cap.
func (r *QueriesRepository) CountFiledSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	n, err := r.q.CountGhostReportsSince(ctx, db.CountGhostReportsSinceParams{
		UserID: userID,
		Since:  pgtype.Timestamptz{Time: since, Valid: true},
	})
	return int(n), err
}

func toReport(row db.GhostReport) Report {
	created := row.CreatedAt.Time
	return Report{
		ID:        row.ID,
		JobID:     row.JobID,
		AppliedOn: row.AppliedOn.Time,
		CreatedAt: &created,
	}
}
