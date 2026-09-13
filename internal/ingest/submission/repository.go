package submission

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries + a pool to the Repository. Each method maps the
// relevant Postgres condition onto a package sentinel: a unique violation on create →
// duplicate pending, no row on get → not found, no row on a status-scoped mark → already
// decided. The pool is needed only by RejectAndBlockHost, which spans two tables and must
// commit or roll back as one action (see its comment).
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
}

// Create inserts a pending submission. The partial unique index on lower(url) WHERE
// status='pending' rejects a second pending submission of the same URL; that surfaces as
// ErrDuplicatePending.
func (r *QueriesRepository) Create(ctx context.Context, submittedBy int64, in moderation.CreateInput) (Submission, error) {
	sub, err := r.q.CreateSubmission(ctx, db.CreateSubmissionParams{
		SubmittedBy:    submittedBy,
		URL:            in.URL,
		Source:         in.Source,
		Title:          in.Title,
		Company:        in.Company,
		Location:       in.Location,
		Remote:         in.Remote,
		Description:    in.Description,
		PostedAt:       pgconv.Timestamptz(in.PostedAt),
		Skills:         in.Skills,
		Regions:        in.Regions,
		Cities:         in.Cities,
		WorkMode:       in.WorkMode,
		EmploymentType: in.EmploymentType,
		Seniority:      in.Seniority,
		SalaryMin:      pgconv.Int4(in.SalaryMin),
		SalaryMax:      pgconv.Int4(in.SalaryMax),
		SalaryCurrency: in.SalaryCurrency,
		SalaryPeriod:   in.SalaryPeriod,
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

// IsHostBlocked reports whether host is on the submission-domain blocklist.
func (r *QueriesRepository) IsHostBlocked(ctx context.Context, host string) (bool, error) {
	return r.q.IsHostBlocked(ctx, host)
}

// RejectAndBlockHost adds host to the blocklist and rejects id plus every other pending
// submission whose URL host normalizes to host, all in one transaction: a moderator
// blocking a spam domain must never leave the blocklist entry live while sibling spam rows
// are still sitting in the queue, or vice versa.
//
// Host matching happens here, in Go, rather than in SQL: normalizeHost's rules (lowercase,
// strip one leading "www.") are the same ones Submit checks against, and duplicating them
// as a second, SQL implementation would risk the two drifting apart.
//
// ListPendingSubmissionURLs carries no LIMIT, unlike the moderator-facing
// ListPendingSubmissions (capped at 500 for display) — a cap here would defeat the point:
// a spammer's whole backlog must be swept, however large, since a row left behind is a row
// still on the board using a domain the moderator just decided to block. This is safe
// because the query only ever runs on this deliberate, infrequent moderator action (never
// on a hot path) and is index-scanned via the partial `status = 'pending'` index rather
// than a full table scan.
func (r *QueriesRepository) RejectAndBlockHost(ctx context.Context, id int64, host string, reviewerID int64, reason string) (Submission, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Submission{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)

	if err := qtx.BlockHost(ctx, db.BlockHostParams{Host: host, BlockedBy: reviewerID, Reason: reason}); err != nil {
		return Submission{}, err
	}

	candidates, err := qtx.ListPendingSubmissionURLs(ctx)
	if err != nil {
		return Submission{}, err
	}
	var ids []int64
	for _, c := range candidates {
		if hostOf(c.URL) == host {
			ids = append(ids, c.ID)
		}
	}

	rejected, err := qtx.MarkSubmissionsRejectedByIDs(ctx, db.MarkSubmissionsRejectedByIDsParams{
		ReviewedBy:   reviewerID,
		ReviewReason: reason,
		Ids:          ids,
	})
	if err != nil {
		return Submission{}, err
	}

	var target *db.JobSubmission
	for i := range rejected {
		if rejected[i].ID == id {
			target = &rejected[i]
			break
		}
	}
	if target == nil {
		// id was not among the rows rejected — it was no longer pending by the time this
		// ran (a concurrent decision), the same race MarkRejected guards against. The
		// deferred Rollback discards the WHOLE transaction on this path, not just id's own
		// status: the blocklist insert and every sibling's bulk reject are undone too, even
		// though nothing was wrong with them. The caller cannot retry this same action
		// (id is now permanently non-pending, so Service.Reject's own pending check fails
		// before ever reaching here) — a moderator hitting this window has to re-target a
		// different still-pending sibling to get the block+bulk-reject to go through.
		return Submission{}, ErrAlreadyDecided
	}

	if err := tx.Commit(ctx); err != nil {
		return Submission{}, err
	}
	return fromRow(*target), nil
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

		Skills:         row.Skills,
		Regions:        row.Regions,
		Cities:         row.Cities,
		WorkMode:       row.WorkMode,
		EmploymentType: row.EmploymentType,
		Seniority:      row.Seniority,
		SalaryMin:      pgconv.IntPtr(row.SalaryMin),
		SalaryMax:      pgconv.IntPtr(row.SalaryMax),
		SalaryCurrency: row.SalaryCurrency,
		SalaryPeriod:   row.SalaryPeriod,
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

			Skills:         row.Skills,
			Regions:        row.Regions,
			Cities:         row.Cities,
			WorkMode:       row.WorkMode,
			EmploymentType: row.EmploymentType,
			Seniority:      row.Seniority,
			SalaryMin:      pgconv.IntPtr(row.SalaryMin),
			SalaryMax:      pgconv.IntPtr(row.SalaryMax),
			SalaryCurrency: row.SalaryCurrency,
			SalaryPeriod:   row.SalaryPeriod,
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

			Skills:         row.Skills,
			Regions:        row.Regions,
			Cities:         row.Cities,
			WorkMode:       row.WorkMode,
			EmploymentType: row.EmploymentType,
			Seniority:      row.Seniority,
			SalaryMin:      pgconv.IntPtr(row.SalaryMin),
			SalaryMax:      pgconv.IntPtr(row.SalaryMax),
			SalaryCurrency: row.SalaryCurrency,
			SalaryPeriod:   row.SalaryPeriod,
		},
		JobSlug: row.JobSlug.String,
	}
}
