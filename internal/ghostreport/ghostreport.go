// Package ghostreport is the lightweight evidence channel behind the ghost job
// signal: one person stating they applied to a posting on a given date and were
// never answered, and withdrawing that statement when the employer finally
// answers.
//
// It is deliberately NOT internal/report. That queue exists so a moderator can
// CLOSE a job, and a claim that is merely evidence cannot be expressed as a
// close: it needs to accumulate, to be counted beside other people's, and to be
// retracted. Nothing here reaches a moderator, and nothing here closes anything.
//
// The service owns validation, the daily cap and retraction; the Repository owns
// persistence, including the two refusals that are structural rather than checks
// — an unverified address and a closed job insert no row at all.
package ghostreport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/userjob"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalid is a content-validation failure (400); its message is user-facing.
	ErrInvalid = errors.New("ghostreport: invalid")
	// ErrDuplicate is a second claim about one job by one person (409).
	ErrDuplicate = errors.New("ghostreport: already reported")
	// ErrNotFound is a retraction of a claim that is not there (404).
	ErrNotFound = errors.New("ghostreport: not found")
	// ErrRateLimited is a filing past the daily cap (429).
	ErrRateLimited = errors.New("ghostreport: too many reports today")
	// ErrJobClosed is a claim about a posting already taken down (409) — there is
	// nothing left to warn anyone about.
	ErrJobClosed = errors.New("ghostreport: job is closed")
	// ErrUnverified is a claim from an account whose address was never proven (403).
	ErrUnverified = errors.New("ghostreport: email not verified")
)

// DailyCap bounds how many claims one account may file per day. A real job
// seeker does not have more than a handful of silent applications to report in a
// day, so the cap costs them nothing while bounding what one account can do to
// the catalogue. It bounds volume; it judges no single claim.
const DailyCap = 20

// Report is a stored claim. AppliedOn is the date the reporter states they
// applied — their word, not an observation, which is why it never reaches the
// tracking board's applied_at.
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
	CountFiledSince(ctx context.Context, userID int64, since time.Time) (int, error)
}

// Service implements the claim use cases.
type Service struct{ repo Repository }

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service { return &Service{repo: repo} }

// File records userID's claim about jobID. `now` is a parameter rather than a
// clock of the service's own, so the date rules are testable without waiting.
//
// It deliberately does NOT refuse a claim too fresh to be evidence. Maturity
// belongs to the aggregator, which reads the silence ladder; refusing here would
// lose the report of somebody who applied three days ago and thought to tell us
// then, and would ask them to come back on a date they will not remember.
func (s *Service) File(ctx context.Context, userID, jobID int64, appliedOn, now time.Time) (Report, error) {
	if err := validateAppliedOn(appliedOn, now); err != nil {
		return Report{}, err
	}
	filed, err := s.repo.CountFiledSince(ctx, userID, now.AddDate(0, 0, -1))
	if err != nil {
		return Report{}, err
	}
	if filed >= DailyCap {
		return Report{}, ErrRateLimited
	}
	return s.repo.Create(ctx, userID, jobID, appliedOn)
}

// Retract withdraws userID's claim about jobID. This is how the signal
// self-heals: an employer who answers on day 40 costs the posting its evidence.
func (s *Service) Retract(ctx context.Context, userID, jobID int64) error {
	return s.repo.Retract(ctx, userID, jobID)
}

// validateAppliedOn bounds the stated date to the window in which it can mean anything: not the
// future, and not so far back that it describes a different hiring round.
//
// The window itself belongs to internal/userjob, which the tracker's dated apply reads too. Only
// the mapping onto this package's 400-sentinel stays here, so a claim and a tracked application
// cannot come to disagree about which dates are believable.
func validateAppliedOn(appliedOn, now time.Time) error {
	if err := userjob.ValidateAppliedOn(appliedOn, now); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	return nil
}
