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
	"time"
	"unicode/utf8"
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
	// ErrInvalidAuthorLabel is an over-long board author label (mapped to 400).
	ErrInvalidAuthorLabel = errors.New("savedsearch: author label must be at most 60 characters")
	// ErrSlugTaken is a public-slug UNIQUE collision on share. It is an internal retry
	// signal (Share regenerates the suffix and tries again), not a client-facing status.
	ErrSlugTaken = errors.New("savedsearch: public slug already taken")
)

const (
	// maxNameLen bounds a saved-search name; the migration's CHECK is the backstop.
	maxNameLen = 100
	// maxPerUser caps how many saved searches a single user may keep.
	maxPerUser = 50
	// maxAuthorLabelLen bounds a board's optional author label.
	maxAuthorLabelLen = 60
)

// SavedSearch is a stored named filter snapshot: the package domain type, decoupled from
// the generated db row. The internal owner column (user_id) is dropped — it is never on the
// wire and scoping is enforced in SQL — while created_at/updated_at are kept as *time.Time
// because the handler serializes them. PublicSlug/AuthorLabel are plain strings, empty when
// the board is private / anonymous (a shared board always carries a non-empty slug, so an
// empty PublicSlug is an unambiguous "not shared").
type SavedSearch struct {
	ID          int64
	Name        string
	Query       string
	PublicSlug  string
	AuthorLabel string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

// Board is the public read of a shared board: only its display fields (no owner columns).
// AuthorLabel is empty when the board is anonymous.
type Board struct {
	Name        string
	Query       string
	AuthorLabel string
}

// Repository is the persistence contract for saved searches. Every method is
// user-scoped. Create maps a unique violation to ErrDuplicateName; Update maps a
// unique violation to ErrDuplicateName and a missing owner-scoped row to ErrNotFound;
// Delete maps "no row affected" to ErrNotFound. Implementations map the generated db
// rows to SavedSearch/Board, so the use case never sees db.*.
type Repository interface {
	List(ctx context.Context, userID int64) ([]SavedSearch, error)
	Count(ctx context.Context, userID int64) (int64, error)
	Create(ctx context.Context, userID int64, name, query string) (SavedSearch, error)
	// Update overwrites the name and/or query (a nil field is left unchanged), owner-scoped.
	Update(ctx context.Context, id, userID int64, name, query *string) (SavedSearch, error)
	Delete(ctx context.Context, id, userID int64) error
	// Get reads one of a user's saved searches, owner-scoped; no row → ErrNotFound.
	Get(ctx context.Context, id, userID int64) (SavedSearch, error)
	// SetPublicSlug publishes a board (owner-scoped); a slug UNIQUE collision →
	// ErrSlugTaken (the service retries), no owner-scoped row → ErrNotFound. An empty
	// authorLabel is stored NULL (anonymous).
	SetPublicSlug(ctx context.Context, id, userID int64, publicSlug, authorLabel string) (SavedSearch, error)
	// ClearPublicSlug unpublishes a board (owner-scoped); no owner-scoped row → ErrNotFound.
	ClearPublicSlug(ctx context.Context, id, userID int64) error
	// GetPublicBoard reads a shared board by slug (no auth, no owner-scoping); no row → ErrNotFound.
	GetPublicBoard(ctx context.Context, slug string) (Board, error)
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
func (s *Service) List(ctx context.Context, userID int64) ([]SavedSearch, error) {
	return s.repo.List(ctx, userID)
}

// Create validates and stores a saved search for the user. The name is trimmed and
// bounded; the per-user cap is checked before the insert; a duplicate name surfaces as
// ErrDuplicateName (mapped by the repository). An empty query is valid — it is the
// unfiltered "show all" view.
func (s *Service) Create(ctx context.Context, userID int64, name, query string) (SavedSearch, error) {
	name, err := validName(name)
	if err != nil {
		return SavedSearch{}, err
	}
	count, err := s.repo.Count(ctx, userID)
	if err != nil {
		return SavedSearch{}, err
	}
	if count >= maxPerUser {
		return SavedSearch{}, ErrCapExceeded
	}
	return s.repo.Create(ctx, userID, name, query)
}

// Update overwrites a saved search's name and/or query, scoped to its owner. A nil field
// is left unchanged (partial update). A provided name is validated; a provided query is
// taken as-is (an empty string is a real "show all" value). A missing or non-owned row
// surfaces as ErrNotFound (mapped by the repository).
func (s *Service) Update(ctx context.Context, userID, id int64, name, query *string) (SavedSearch, error) {
	if name != nil {
		valid, err := validName(*name)
		if err != nil {
			return SavedSearch{}, err
		}
		name = &valid
	}
	return s.repo.Update(ctx, id, userID, name, query)
}

// Delete removes one of the user's saved searches. A missing or non-owned row surfaces as
// ErrNotFound (mapped by the repository).
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, id, userID)
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
