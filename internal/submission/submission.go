// Package submission is the public job-submission queue: any authenticated user can
// submit a vacancy for review, and a moderator approves it into the live catalogue or
// rejects it. It owns validation (shared with internal/moderation) and the review state
// machine; the Repository owns persistence. Approval mints the live job by delegating to
// the moderation use case (the Minter), so derivation, dedup, and the enrichment enqueue
// are not duplicated here.
package submission

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/moderation"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrSubmissionNotFound is the missing review target (mapped to 404).
	ErrSubmissionNotFound = errors.New("submission: not found")
	// ErrDuplicatePending is a second submission of a URL already awaiting review
	// (the partial unique index; mapped to 409).
	ErrDuplicatePending = errors.New("submission: a pending submission for this URL already exists")
	// ErrAlreadyDecided is an approve/reject of a submission that is no longer pending
	// (mapped to 409).
	ErrAlreadyDecided = errors.New("submission: already decided")
)

// Minter mints a live vacancy from validated content. internal/moderation.Service
// satisfies it; the seam keeps the service testable without a database.
type Minter interface {
	Create(ctx context.Context, actorID int64, in moderation.CreateInput) (db.Job, error)
}

// Repository is the persistence contract for the submission queue.
type Repository interface {
	Create(ctx context.Context, p db.CreateSubmissionParams) (db.JobSubmission, error)
	Get(ctx context.Context, id int64) (db.JobSubmission, error)
	ListPending(ctx context.Context) ([]db.ListPendingSubmissionsRow, error)
	ListByUser(ctx context.Context, userID int64) ([]db.ListSubmissionsByUserRow, error)
	MarkApproved(ctx context.Context, p db.MarkSubmissionApprovedParams) (db.JobSubmission, error)
	MarkRejected(ctx context.Context, p db.MarkSubmissionRejectedParams) (db.JobSubmission, error)
}

// Service implements the submission use cases.
type Service struct {
	repo   Repository
	minter Minter
}

// New creates a Service backed by the given Repository and Minter.
func New(repo Repository, minter Minter) *Service {
	return &Service{repo: repo, minter: minter}
}

// Submit validates contributed content against the same contract a moderator create uses
// and stores it as a pending submission owned by the given user. A second submission of a
// URL already pending surfaces ErrDuplicatePending (the repository maps the unique
// violation).
func (s *Service) Submit(ctx context.Context, submittedBy int64, in moderation.CreateInput) (db.JobSubmission, error) {
	if err := in.Validate(); err != nil {
		return db.JobSubmission{}, err
	}
	return s.repo.Create(ctx, db.CreateSubmissionParams{
		SubmittedBy: submittedBy,
		URL:         in.URL,
		Source:      in.Source,
		Title:       in.Title,
		Company:     in.Company,
		Location:    in.Location,
		Remote:      in.Remote,
		Description: in.Description,
		PostedAt:    toTimestamptz(in.PostedAt),
	})
}

// ListMine returns the given user's submissions, newest first. Each row carries the
// minted job's slug (when approved) so the UI can link to the live vacancy.
func (s *Service) ListMine(ctx context.Context, userID int64) ([]db.ListSubmissionsByUserRow, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListPending returns the moderator review queue (with submitter emails), newest first.
func (s *Service) ListPending(ctx context.Context) ([]db.ListPendingSubmissionsRow, error) {
	return s.repo.ListPending(ctx)
}

// Approve mints a live vacancy from a pending submission's fields (attributed to the
// submitter) and marks the submission approved, recording the reviewing moderator and the
// minted job. A missing submission is ErrSubmissionNotFound; one that is no longer pending
// is ErrAlreadyDecided. The mint runs before the mark; because the moderation upsert is
// idempotent on the URL, a failure between the two is safe to retry.
func (s *Service) Approve(ctx context.Context, reviewerID, id int64) (db.JobSubmission, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return db.JobSubmission{}, err
	}
	if sub.Status != statusPending {
		return db.JobSubmission{}, ErrAlreadyDecided
	}
	job, err := s.minter.Create(ctx, sub.SubmittedBy, moderation.CreateInput{
		URL:         sub.URL,
		Source:      sub.Source,
		Title:       sub.Title,
		Company:     sub.Company,
		Location:    sub.Location,
		Remote:      sub.Remote,
		Description: sub.Description,
		PostedAt:    fromTimestamptz(sub.PostedAt),
	})
	if err != nil {
		return db.JobSubmission{}, err
	}
	return s.repo.MarkApproved(ctx, db.MarkSubmissionApprovedParams{
		ID:         id,
		ReviewedBy: reviewerID,
		JobID:      job.ID,
	})
}

// Reject marks a pending submission rejected with an optional reason, recording the
// reviewing moderator. No job is created. A missing submission is ErrSubmissionNotFound;
// one that is no longer pending is ErrAlreadyDecided.
func (s *Service) Reject(ctx context.Context, reviewerID, id int64, reason string) (db.JobSubmission, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return db.JobSubmission{}, err
	}
	if sub.Status != statusPending {
		return db.JobSubmission{}, ErrAlreadyDecided
	}
	return s.repo.MarkRejected(ctx, db.MarkSubmissionRejectedParams{
		ID:           id,
		ReviewedBy:   reviewerID,
		ReviewReason: reason,
	})
}

// statusPending is the only status that can be approved or rejected; the closed vocabulary
// lives in the migration's CHECK.
const statusPending = "pending"

// toTimestamptz maps an optional time to the pgtype the params expect; nil becomes NULL.
func toTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// fromTimestamptz maps a nullable DB timestamp back to an optional time for the mint input.
func fromTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
