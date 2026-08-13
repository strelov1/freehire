package screeninganswers

import (
	"context"
	"errors"
)

// ErrNotFound is the caller having stated no screening answer yet.
var ErrNotFound = errors.New("screeninganswers: not found")

// ValidationError is the input itself being rejected — an unrecognized country code, a
// malformed currency, an out-of-vocabulary period, a non-positive salary, a negative
// notice period — as opposed to a Repository failure. A caller (the HTTP handler, the
// assistant tool) needs the distinction to answer a bad request with 400 and a genuine
// fault with 500, rather than treating every error Update can return the same way.
type ValidationError struct {
	err error
}

func (e *ValidationError) Error() string { return e.err.Error() }
func (e *ValidationError) Unwrap() error { return e.err }

// Repository is the persistence contract for the single per-user screening-answers
// record. Get maps a missing row to ErrNotFound; Upsert creates or replaces.
// Implementations map the generated db row to Answers, so the use case never sees db.*.
type Repository interface {
	Get(ctx context.Context, userID int64) (Answers, error)
	Upsert(ctx context.Context, userID int64, a Answers) (Answers, error)
}

// Store implements the screening-answers use case: read the caller's record, and
// partially update it under Sanitize/Validate.
type Store struct {
	repo Repository
}

// New creates a Store backed by the given Repository.
func New(repo Repository) *Store {
	return &Store{repo: repo}
}

// Get returns the caller's screening answers, or ErrNotFound when they have stated none
// yet.
func (s *Store) Get(ctx context.Context, userID int64) (Answers, error) {
	return s.repo.Get(ctx, userID)
}

// Update sanitizes and validates the given fields, merges them over whatever the caller
// already has stored (a field the update leaves unset keeps its stored value), and
// persists the merged record. A caller with no existing record is treated as starting
// from a fully unstated one, so the first update is also a create.
func (s *Store) Update(ctx context.Context, userID int64, update Answers) (Answers, error) {
	update.Sanitize()
	if err := update.Validate(); err != nil {
		return Answers{}, &ValidationError{err: err}
	}

	existing, err := s.repo.Get(ctx, userID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Answers{}, err
	}

	return s.repo.Upsert(ctx, userID, Merge(existing, update))
}
