package screeninganswers

import (
	"context"
	"errors"
	"fmt"
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
// record. Implementations map the generated db row to Answers, so the use case never sees
// db.*.
//
// Get maps a missing row to ErrNotFound — a plain, unlocked read for callers (Store.Get)
// that only display the record.
//
// UpdateLocked is the only write path, and it is atomic by construction: implementations
// must take an exclusive lock on the caller's row for the whole operation (a no-op when no
// row exists yet), call merge exactly once with what that locked read found (a fully
// unstated Answers{} when there is no row), and persist whatever merge returns before
// releasing the lock. That is what serializes two concurrent Update calls for the same
// userID onto one, race-free read-merge-write instead of each reading independently and
// whichever writes second clobbering the first (a lost update).
type Repository interface {
	Get(ctx context.Context, userID int64) (Answers, error)
	UpdateLocked(ctx context.Context, userID int64, merge func(existing Answers) Answers) (Answers, error)
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
//
// The read, merge and write happen as one atomic operation inside UpdateLocked (a row
// lock held for the whole transaction), not as separate Get/Upsert calls: two Updates
// racing for the same userID — the manual-edit handler and the assistant's
// screening_answers_set tool both call this same method — would otherwise both read the
// same "existing" record and the one that commits second would silently overwrite the
// first's fields instead of merging onto them.
//
// Each field in cleared is withdrawn back to unstated (see Merge). A field the update both
// states and clears is a contradiction and refused rather than resolved either way.
func (s *Store) Update(ctx context.Context, userID int64, update Answers, cleared ...Field) (Answers, error) {
	update.Sanitize()
	if err := update.Validate(); err != nil {
		return Answers{}, &ValidationError{err: err}
	}
	for _, f := range cleared {
		if update.states(f) {
			return Answers{}, &ValidationError{err: fmt.Errorf("%s: both stated and cleared", f)}
		}
	}

	return s.repo.UpdateLocked(ctx, userID, func(existing Answers) Answers {
		return Merge(existing, update, cleared...)
	})
}
