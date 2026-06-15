// Package report is the job-report moderation queue: any authenticated user can flag a
// live vacancy with a reason and details, and a moderator resolves the report (optionally
// soft-closing the job) or dismisses it. It owns reason validation and the review state
// machine; the Repository owns persistence. A report never creates a job — unlike the
// submission queue there is no Minter; resolving may close the reported job through the
// narrow JobCloser seam, so the close concern stays explicit and testable.
package report

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/db"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrReportNotFound is the missing review target (mapped to 404).
	ErrReportNotFound = errors.New("report: not found")
	// ErrDuplicateOpen is a second open report of the same job by the same user (the
	// partial unique index; mapped to 409).
	ErrDuplicateOpen = errors.New("report: an open report for this job already exists")
	// ErrAlreadyDecided is a resolve/dismiss of a report that is no longer pending
	// (mapped to 409).
	ErrAlreadyDecided = errors.New("report: already decided")
	// ErrInvalid is a content-validation failure (mapped to 400); its message is
	// user-facing.
	ErrInvalid = errors.New("report: invalid")
)

// maxDetailsLen bounds the free-text explanation so a single report cannot carry an
// unbounded payload (the content-field convention).
const maxDetailsLen = 5000

// validReasons is the closed reason vocabulary, mirrored by the migration's CHECK and the
// SPA. The service re-validates against it so an out-of-vocabulary value is a clean 400
// rather than a raw constraint error.
var validReasons = map[string]bool{
	"no_response":  true,
	"not_relevant": true,
	"spam":         true,
	"fraud":        true,
	"other":        true,
}

// FileInput is the user-supplied content of a report. The job is identified separately (by
// the slug the handler resolves), so it is not part of this shape.
type FileInput struct {
	Reason          string
	Details         string
	ContactTelegram string
}

// validate normalizes and checks the input, returning a trimmed copy ready to persist. A
// reason outside the vocabulary, blank details, or over-long details is ErrInvalid.
func (in FileInput) validate() (FileInput, error) {
	if !validReasons[in.Reason] {
		return FileInput{}, fmt.Errorf("%w: unknown reason %q", ErrInvalid, in.Reason)
	}
	details := strings.TrimSpace(in.Details)
	if details == "" {
		return FileInput{}, fmt.Errorf("%w: details are required", ErrInvalid)
	}
	if len(details) > maxDetailsLen {
		return FileInput{}, fmt.Errorf("%w: details too long", ErrInvalid)
	}
	return FileInput{
		Reason:          in.Reason,
		Details:         details,
		ContactTelegram: strings.TrimSpace(in.ContactTelegram),
	}, nil
}

// Repository is the persistence contract for the report queue.
type Repository interface {
	Create(ctx context.Context, p db.CreateReportParams) (db.JobReport, error)
	Get(ctx context.Context, id int64) (db.JobReport, error)
	ListPending(ctx context.Context) ([]db.ListPendingReportsRow, error)
	MarkResolved(ctx context.Context, p db.MarkReportResolvedParams) (db.JobReport, error)
	MarkDismissed(ctx context.Context, p db.MarkReportDismissedParams) (db.JobReport, error)
}

// JobCloser soft-closes one job. The QueriesRepository satisfies it (over CloseJobByID);
// the seam keeps Resolve testable without a database and keeps the close path explicit.
type JobCloser interface {
	Close(ctx context.Context, jobID int64) error
}

// Service implements the report use cases.
type Service struct {
	repo   Repository
	closer JobCloser
}

// New creates a Service backed by the given Repository and JobCloser.
func New(repo Repository, closer JobCloser) *Service {
	return &Service{repo: repo, closer: closer}
}

// File validates the content and stores a pending report owned by reportedBy against
// jobID. A second open report of the same job by the same user surfaces ErrDuplicateOpen
// (the repository maps the unique violation).
func (s *Service) File(ctx context.Context, reportedBy, jobID int64, in FileInput) (db.JobReport, error) {
	v, err := in.validate()
	if err != nil {
		return db.JobReport{}, err
	}
	return s.repo.Create(ctx, db.CreateReportParams{
		ReportedBy:      reportedBy,
		JobID:           jobID,
		Reason:          v.Reason,
		Details:         v.Details,
		ContactTelegram: v.ContactTelegram,
	})
}

// ListPending returns the moderator review queue (with reporter email and job slug/title),
// newest first.
func (s *Service) ListPending(ctx context.Context) ([]db.ListPendingReportsRow, error) {
	return s.repo.ListPending(ctx)
}

// Resolve marks a pending report resolved, recording the reviewing moderator. When closeJob
// is set, the reported job is soft-closed first; a close failure aborts before the mark, so
// the report stays pending and the action is safe to retry. A missing report is
// ErrReportNotFound; one no longer pending is ErrAlreadyDecided.
func (s *Service) Resolve(ctx context.Context, reviewerID, id int64, closeJob bool) (db.JobReport, error) {
	rep, err := s.repo.Get(ctx, id)
	if err != nil {
		return db.JobReport{}, err
	}
	if rep.Status != statusPending {
		return db.JobReport{}, ErrAlreadyDecided
	}
	if closeJob {
		if err := s.closer.Close(ctx, rep.JobID); err != nil {
			return db.JobReport{}, err
		}
	}
	return s.repo.MarkResolved(ctx, db.MarkReportResolvedParams{ID: id, ReviewedBy: reviewerID})
}

// Dismiss marks a pending report dismissed with an optional reason, recording the reviewing
// moderator. The reported job is not touched. A missing report is ErrReportNotFound; one no
// longer pending is ErrAlreadyDecided.
func (s *Service) Dismiss(ctx context.Context, reviewerID, id int64, reason string) (db.JobReport, error) {
	rep, err := s.repo.Get(ctx, id)
	if err != nil {
		return db.JobReport{}, err
	}
	if rep.Status != statusPending {
		return db.JobReport{}, ErrAlreadyDecided
	}
	return s.repo.MarkDismissed(ctx, db.MarkReportDismissedParams{
		ID:           id,
		ReviewedBy:   reviewerID,
		ReviewReason: reason,
	})
}

// statusPending is the only status that can be resolved or dismissed; the closed vocabulary
// lives in the migration's CHECK.
const statusPending = "pending"
