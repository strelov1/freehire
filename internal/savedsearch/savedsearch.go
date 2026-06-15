// Package savedsearch is the per-user saved-search use case: a signed-in user names a
// snapshot of their job-search filter state (the canonical search query string) and can
// list, re-apply, overwrite, and delete those snapshots. It owns validation (name bounds,
// the per-user cap); the Repository owns persistence and maps the relevant Postgres
// conditions (unique violation, no row) onto the package sentinels.
package savedsearch

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidName is a blank or over-long name (mapped to 400).
	ErrInvalidName = errors.New("savedsearch: name must be 1-100 characters")
	// ErrDuplicateName is a name the user already uses (the UNIQUE (user_id, name)
	// constraint; mapped to 409).
	ErrDuplicateName = errors.New("savedsearch: a saved search with this name already exists")
	// ErrCapExceeded is a create past the per-user limit (mapped to 409).
	ErrCapExceeded = errors.New("savedsearch: saved-search limit reached")
	// ErrNotFound is a missing or non-owned target (mapped to 404).
	ErrNotFound = errors.New("savedsearch: not found")
)

const (
	// maxNameLen bounds a saved-search name; the migration's CHECK is the backstop.
	maxNameLen = 100
	// maxPerUser caps how many saved searches a single user may keep.
	maxPerUser = 50
)

// Repository is the persistence contract for saved searches. Every method is
// user-scoped. Create maps a unique violation to ErrDuplicateName; Update maps a
// unique violation to ErrDuplicateName and a missing owner-scoped row to ErrNotFound;
// Delete maps "no row affected" to ErrNotFound.
type Repository interface {
	List(ctx context.Context, userID int64) ([]db.SavedSearch, error)
	Count(ctx context.Context, userID int64) (int64, error)
	Create(ctx context.Context, p db.CreateSavedSearchParams) (db.SavedSearch, error)
	Update(ctx context.Context, p db.UpdateSavedSearchParams) (db.SavedSearch, error)
	Delete(ctx context.Context, p db.DeleteSavedSearchParams) error
}

// Service implements the saved-search use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns the user's saved searches, most recently updated first.
func (s *Service) List(ctx context.Context, userID int64) ([]db.SavedSearch, error) {
	return s.repo.List(ctx, userID)
}

// Create validates and stores a saved search for the user. The name is trimmed and
// bounded; the per-user cap is checked before the insert; a duplicate name surfaces as
// ErrDuplicateName (mapped by the repository). An empty query is valid — it is the
// unfiltered "show all" view.
func (s *Service) Create(ctx context.Context, userID int64, name, query string) (db.SavedSearch, error) {
	name, err := validName(name)
	if err != nil {
		return db.SavedSearch{}, err
	}
	count, err := s.repo.Count(ctx, userID)
	if err != nil {
		return db.SavedSearch{}, err
	}
	if count >= maxPerUser {
		return db.SavedSearch{}, ErrCapExceeded
	}
	return s.repo.Create(ctx, db.CreateSavedSearchParams{UserID: userID, Name: name, Query: query})
}

// Update overwrites a saved search's name and/or query, scoped to its owner. A nil field
// is left unchanged (partial update). A provided name is validated; a provided query is
// taken as-is (an empty string is a real "show all" value). A missing or non-owned row
// surfaces as ErrNotFound (mapped by the repository).
func (s *Service) Update(ctx context.Context, userID, id int64, name, query *string) (db.SavedSearch, error) {
	p := db.UpdateSavedSearchParams{ID: id, UserID: userID}
	if name != nil {
		valid, err := validName(*name)
		if err != nil {
			return db.SavedSearch{}, err
		}
		p.Name = pgtype.Text{String: valid, Valid: true}
	}
	if query != nil {
		p.Query = pgtype.Text{String: *query, Valid: true}
	}
	return s.repo.Update(ctx, p)
}

// Delete removes one of the user's saved searches. A missing or non-owned row surfaces as
// ErrNotFound (mapped by the repository).
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, db.DeleteSavedSearchParams{ID: id, UserID: userID})
}

// validName trims the name and enforces the 1..maxNameLen bound (counted in runes, to
// match the DB CHECK's character semantics — names are often Cyrillic), returning the
// trimmed value or ErrInvalidName.
func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return "", ErrInvalidName
	}
	return name, nil
}
