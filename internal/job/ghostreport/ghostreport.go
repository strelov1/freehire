// Package ghostreport is the lightweight evidence channel behind the ghost job
// signal: one person stating they applied to a posting on a given date and were
// never answered, and withdrawing that statement when the employer finally
// answers.
//
// It is not internal/engage/report, whose queue lets a moderator close a job. A claim that
// is merely evidence must accumulate, be counted beside others, and be retractable;
// nothing here reaches a moderator or closes anything.
//
// The service owns validation and retraction; the Repository owns persistence, including
// the structural refusals: an unverified address and a closed job insert no row.
package ghostreport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/job/applydate"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalid is a content-validation failure (400); its message is user-facing.
	ErrInvalid = errors.New("ghostreport: invalid")
	// ErrDuplicate is a second claim about one job by one person (409).
	ErrDuplicate = errors.New("ghostreport: already reported")
	// ErrNotFound is a retraction of a claim that is not there (404).
	ErrNotFound = errors.New("ghostreport: not found")
	// ErrJobClosed is a claim about a posting already taken down (409).
	ErrJobClosed = errors.New("ghostreport: job is closed")
	// ErrUnverified is a claim from an account whose address was never proven (403).
	ErrUnverified = errors.New("ghostreport: email not verified")
)

// Report is a stored claim. AppliedOn is the reporter's word, not an observation, so it
// never reaches the tracking board's applied_at.
type Report struct {
	ID        int64
	JobID     int64
	AppliedOn time.Time
	CreatedAt *time.Time
}

// Repository is the persistence contract. Create surfaces ErrDuplicate,
// ErrUnverified and ErrJobClosed; Retract surfaces ErrNotFound.
type Repository interface {
	Create(ctx context.Context, userID, jobID int64, appliedOn time.Time) (Report, error)
	Retract(ctx context.Context, userID, jobID int64) error
}

// Service implements the claim use cases.
type Service struct{ repo Repository }

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service { return &Service{repo: repo} }

// File records userID's claim about jobID. It does not refuse a claim too fresh to be
// evidence: maturity belongs to the aggregator, and refusing here would lose the report.
func (s *Service) File(ctx context.Context, userID, jobID int64, appliedOn, now time.Time) (Report, error) {
	if err := validateAppliedOn(appliedOn, now); err != nil {
		return Report{}, err
	}
	return s.repo.Create(ctx, userID, jobID, appliedOn)
}

// Retract withdraws userID's claim about jobID, so the signal self-heals when an employer
// finally answers.
func (s *Service) Retract(ctx context.Context, userID, jobID int64) error {
	return s.repo.Retract(ctx, userID, jobID)
}

// validateAppliedOn bounds the stated date to the window where it can mean anything: not
// the future, not a different hiring round. The window lives in applydate, shared with the
// tracker's dated apply; only the mapping onto ErrInvalid stays here.
func validateAppliedOn(appliedOn, now time.Time) error {
	if err := applydate.Validate(appliedOn, now); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	return nil
}
