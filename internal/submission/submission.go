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

	"github.com/strelov1/freehire/internal/job"
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

// Submission is a stored queue entry: the package domain type, decoupled from the generated
// db row. SubmittedBy is kept (unlike the report queue's dropped ownership columns) because
// Approve mints the live job attributed to the original submitter; it is never on the wire.
// created_at/reviewed_at/posted_at are *time.Time because the handler serializes them.
type Submission struct {
	ID           int64
	SubmittedBy  int64
	URL          string
	Source       string
	Title        string
	Company      string
	Location     string
	Remote       bool
	Description  string
	PostedAt     *time.Time
	Status       string
	ReviewReason string
	ReviewedAt   *time.Time
	CreatedAt    *time.Time

	// The structured facets the submitter stated, retained so the moderator sees them
	// and Approve can carry them onto the minted job (see the Approve mint below).
	Skills         []string
	Regions        []string
	Cities         []string
	WorkMode       string
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

// PendingSubmission is a moderator-queue row: a Submission plus the joined submitter email
// the reviewer needs.
type PendingSubmission struct {
	Submission
	SubmitterEmail string
}

// UserSubmission is a "my submissions" row: a Submission plus the minted job's public slug,
// set only on an approved submission (empty otherwise) so the UI can link to /jobs/<slug>.
type UserSubmission struct {
	Submission
	JobSlug string
}

// Minter mints a live vacancy from validated content. internal/moderation.Service
// satisfies it; the seam keeps the service testable without a database.
type Minter interface {
	Create(ctx context.Context, actorID int64, in moderation.CreateInput) (job.Job, job.Extras, error)
}

// Repository is the persistence contract for the submission queue, expressed in the package
// domain types rather than the generated db rows. Create takes the validated content plus the
// owning user; the mark methods take primitive ids; the adapter builds the db params and maps
// the rows back.
type Repository interface {
	Create(ctx context.Context, submittedBy int64, in moderation.CreateInput) (Submission, error)
	Get(ctx context.Context, id int64) (Submission, error)
	ListPending(ctx context.Context) ([]PendingSubmission, error)
	ListByUser(ctx context.Context, userID int64) ([]UserSubmission, error)
	MarkApproved(ctx context.Context, id, reviewerID, jobID int64) (Submission, error)
	MarkRejected(ctx context.Context, id, reviewerID int64, reason string) (Submission, error)
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
func (s *Service) Submit(ctx context.Context, submittedBy int64, in moderation.CreateInput) (Submission, error) {
	if err := in.Validate(); err != nil {
		return Submission{}, err
	}
	return s.repo.Create(ctx, submittedBy, in)
}

// ListMine returns the given user's submissions, newest first. Each row carries the
// minted job's slug (when approved) so the UI can link to the live vacancy.
func (s *Service) ListMine(ctx context.Context, userID int64) ([]UserSubmission, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListPending returns the moderator review queue (with submitter emails), newest first.
func (s *Service) ListPending(ctx context.Context) ([]PendingSubmission, error) {
	return s.repo.ListPending(ctx)
}

// Approve mints a live vacancy from a pending submission's fields (attributed to the
// submitter) and marks the submission approved, recording the reviewing moderator and the
// minted job. A missing submission is ErrSubmissionNotFound; one that is no longer pending
// is ErrAlreadyDecided. The mint runs before the mark; because the moderation upsert is
// idempotent on the URL, a failure between the two is safe to retry.
func (s *Service) Approve(ctx context.Context, reviewerID, id int64) (Submission, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return Submission{}, err
	}
	if sub.Status != statusPending {
		return Submission{}, ErrAlreadyDecided
	}
	mintedJob, _, err := s.minter.Create(ctx, sub.SubmittedBy, moderation.CreateInput{
		URL:            sub.URL,
		Source:         sub.Source,
		Title:          sub.Title,
		Company:        sub.Company,
		Location:       sub.Location,
		Remote:         sub.Remote,
		Description:    sub.Description,
		PostedAt:       sub.PostedAt,
		Skills:         sub.Skills,
		Regions:        sub.Regions,
		Cities:         sub.Cities,
		WorkMode:       sub.WorkMode,
		SalaryMin:      sub.SalaryMin,
		SalaryMax:      sub.SalaryMax,
		SalaryCurrency: sub.SalaryCurrency,
		SalaryPeriod:   sub.SalaryPeriod,
	})
	if err != nil {
		return Submission{}, err
	}
	return s.repo.MarkApproved(ctx, id, reviewerID, mintedJob.Fields().ID)
}

// Reject marks a pending submission rejected with an optional reason, recording the
// reviewing moderator. No job is created. A missing submission is ErrSubmissionNotFound;
// one that is no longer pending is ErrAlreadyDecided.
func (s *Service) Reject(ctx context.Context, reviewerID, id int64, reason string) (Submission, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return Submission{}, err
	}
	if sub.Status != statusPending {
		return Submission{}, ErrAlreadyDecided
	}
	return s.repo.MarkRejected(ctx, id, reviewerID, reason)
}

// statusPending is the only status that can be approved or rejected; the closed vocabulary
// lives in the migration's CHECK.
const statusPending = "pending"
