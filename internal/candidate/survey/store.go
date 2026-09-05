package survey

import (
	"context"
	"errors"
)

// ErrNotFound is the Repository's way of saying there is no stored row. It stops at the
// Store: Get turns it into a fully-unstated record, because "has answered nothing" and
// "has no row" are the same fact to every caller, and making them handle both would be
// asking them to care about storage.
var ErrNotFound = errors.New("survey: not found")

// ValidationError is the input itself being rejected — an out-of-vocabulary stage or
// challenge, a note beside a coded challenge, a malformed currency, a non-positive income —
// as opposed to a Repository failure. The HTTP handler needs the distinction to answer a
// bad request with 400 and a genuine fault with 500.
type ValidationError struct {
	err error
}

func (e *ValidationError) Error() string { return e.err.Error() }
func (e *ValidationError) Unwrap() error { return e.err }

// Repository is the persistence contract for the single per-user survey record. Get maps a
// missing row to ErrNotFound; Upsert creates or replaces. Implementations map the generated
// db row to Responses, so the use case never sees db.*.
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

// Update sanitizes and validates the given fields, merges them over whatever the caller
// already has stored (a field the update leaves unset keeps its stored value), and persists
// the merged record. A caller with no existing record starts from a fully unstated one, so
// the first update is also a create.
func (s *Store) Update(ctx context.Context, userID int64, update Responses) (Responses, error) {
	update.Sanitize()

	existing, err := s.repo.Get(ctx, userID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Responses{}, err
	}

	// Validate the MERGED record, not the patch. Validating the patch alone would break the
	// contract one field above it: the note's gate reads the challenge, so a caller who has
	// already stored `other` and now sends only the note — completing an answer, not
	// contradicting one — would be rejected for carrying no challenge, even though "a field
	// the body omits keeps its stored value" says it does not have to.
	merged := Merge(existing, update)
	if err := merged.Validate(); err != nil {
		return Responses{}, &ValidationError{err: err}
	}

	return s.repo.Upsert(ctx, userID, merged)
}
