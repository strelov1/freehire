package survey

import (
	"context"
	"errors"
)

// ErrNotFound means there is no stored row. It stops at the Store: Get turns it into an
// unstated record, since "answered nothing" and "no row" are the same to callers.
var ErrNotFound = errors.New("survey: not found")

// ValidationError is the input itself being rejected (out-of-vocabulary value, note
// beside a coded challenge, malformed currency, non-positive income), as opposed to a
// Repository failure; the handler maps it to 400 and the rest to 500.
type ValidationError struct {
	err error
}

func (e *ValidationError) Error() string { return e.err.Error() }
func (e *ValidationError) Unwrap() error { return e.err }

// Repository is the persistence contract for the per-user survey record. Get maps a
// missing row to ErrNotFound; Upsert creates or replaces.
type Repository interface {
	Get(ctx context.Context, userID int64) (Responses, error)
	Upsert(ctx context.Context, userID int64, a Responses) (Responses, error)
}

// Store implements the survey use case: read the caller's record, and partially update it
// under Sanitize/Validate.
type Store struct {
	repo Repository
}

// New creates a Store backed by the given Repository.
func New(repo Repository) *Store {
	return &Store{repo: repo}
}

// Get returns the caller's survey answers, or a fully-unstated record when they have
// answered nothing. A genuine repository failure still surfaces.
func (s *Store) Get(ctx context.Context, userID int64) (Responses, error) {
	a, err := s.repo.Get(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return Responses{}, nil
	}
	if err != nil {
		return Responses{}, err
	}
	return a, nil
}

// Update sanitizes the given fields, merges them over the stored record (an unset field
// keeps its stored value), validates and persists the result. The first update is also a create.
func (s *Store) Update(ctx context.Context, userID int64, update Responses) (Responses, error) {
	update.Sanitize()

	existing, err := s.repo.Get(ctx, userID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Responses{}, err
	}

	// Validate the MERGED record, not the patch: the note's gate reads the challenge, so a
	// caller who stored `other` and now sends only the note must not be rejected.
	merged := Merge(existing, update)
	if err := merged.Validate(); err != nil {
		return Responses{}, &ValidationError{err: err}
	}

	return s.repo.Upsert(ctx, userID, merged)
}
