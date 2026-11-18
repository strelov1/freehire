// Package savedsearch is the per-user saved-search use case: a signed-in user names a snapshot of
// their job-search filter state (the canonical search query string) and can list, re-apply,
// overwrite and delete it. It owns validation (name bounds, per-user cap); the Repository owns
// persistence and maps Postgres conditions onto the package sentinels.
package savedsearch

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/search/search"
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
	// ErrQueryTooLong is a query string over maxQueryLen (mapped to 400).
	ErrQueryTooLong = errors.New("savedsearch: query is too long")
	// ErrProfileSearchExists is a create with derivedFromProfile=true when the user
	// already has one (the partial UNIQUE (user_id) WHERE derived_from_profile;
	// mapped to 409).
	ErrProfileSearchExists = errors.New("savedsearch: a profile-derived search already exists")
)

const (
	// maxNameLen bounds a saved-search name; the migration's CHECK is the backstop.
	maxNameLen = 100
	// maxPerUser caps how many saved searches a single user may keep.
	maxPerUser = 50
	// maxQueryLen bounds the stored query string. 4000 leaves headroom for a rich filter (a
	// profile-derived query with many skills) while keeping the per-pass url.ParseQuery in
	// internal/engage/notify cheap. The migration's CHECK is the backstop.
	maxQueryLen = 4000
)

// SavedSearch is a stored named filter snapshot, decoupled from the generated db row. The owner
// column (user_id) is dropped; created_at/updated_at are kept as *time.Time for the handler.
type SavedSearch struct {
	ID                 int64
	Name               string
	Query              string
	DerivedFromProfile bool
	CreatedAt          *time.Time
	UpdatedAt          *time.Time
}

// Repository is the persistence contract for saved searches. Every method is user-scoped.
// Unique violations map to ErrDuplicateName, a missing owner-scoped row to ErrNotFound, and
// implementations return SavedSearch so the use case never sees db.*.
type Repository interface {
	List(ctx context.Context, userID int64) ([]SavedSearch, error)
	Count(ctx context.Context, userID int64) (int64, error)
	Create(ctx context.Context, userID int64, name, query string, derivedFromProfile bool) (SavedSearch, error)
	// Update overwrites the name and/or query (a nil field is left unchanged), owner-scoped.
	Update(ctx context.Context, id, userID int64, name, query *string) (SavedSearch, error)
	Delete(ctx context.Context, id, userID int64) error
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

// Create validates and stores a saved search. The name is trimmed and bounded, and the per-user
// cap is checked before the insert. An empty query is valid: the unfiltered "show all" view.
func (s *Service) Create(ctx context.Context, userID int64, name, query string, derivedFromProfile bool) (SavedSearch, error) {
	name, err := validName(name)
	if err != nil {
		return SavedSearch{}, err
	}
	if err := validQuery(query); err != nil {
		return SavedSearch{}, err
	}
	count, err := s.repo.Count(ctx, userID)
	if err != nil {
		return SavedSearch{}, err
	}
	if count >= maxPerUser {
		return SavedSearch{}, ErrCapExceeded
	}
	return s.repo.Create(ctx, userID, name, query, derivedFromProfile)
}

// Update overwrites a saved search's name and/or query, scoped to its owner. A nil field is left
// unchanged; provided values are validated, and an empty query is a real "show all" value.
func (s *Service) Update(ctx context.Context, userID, id int64, name, query *string) (SavedSearch, error) {
	if name != nil {
		valid, err := validName(*name)
		if err != nil {
			return SavedSearch{}, err
		}
		name = &valid
	}
	if query != nil {
		if err := validQuery(*query); err != nil {
			return SavedSearch{}, err
		}
	}
	return s.repo.Update(ctx, id, userID, name, query)
}

// Delete removes one of the user's saved searches; a missing or non-owned row is ErrNotFound.
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, id, userID)
}

// validName trims the name and enforces 1..maxNameLen runes (rune-counted to match the DB CHECK;
// names are often Cyrillic).
func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return "", ErrInvalidName
	}
	return name, nil
}

// validQuery bounds the stored query and checks its free-text syntax with the parser search and the
// notify matcher use. The stored filter state itself stays unchanged.
func validQuery(query string) error {
	if utf8.RuneCountInString(query) > maxQueryLen {
		return ErrQueryTooLong
	}
	// Match notify's reading of partially malformed state: keep every pair url.ParseQuery can read,
	// then validate the q that will execute.
	values, _ := url.ParseQuery(query)
	_, err := search.ParseQuery(values.Get("q"))
	return err
}
