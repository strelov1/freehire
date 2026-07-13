package savedsearch

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository. It maps the relevant Postgres
// conditions onto package sentinels: a unique violation on create/update → duplicate
// name, no row on an owner-scoped update → not found, no row affected on delete → not
// found.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// List returns a user's saved searches, most recently updated first.
func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]SavedSearch, error) {
	rows, err := r.q.ListSavedSearches(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SavedSearch, len(rows))
	for i, row := range rows {
		out[i] = fromRow(row)
	}
	return out, nil
}

// Count returns how many saved searches the user has (the cap check input).
func (r *QueriesRepository) Count(ctx context.Context, userID int64) (int64, error) {
	return r.q.CountSavedSearches(ctx, userID)
}

// Create inserts a saved search, mapping the UNIQUE (user_id, name) violation to
// ErrDuplicateName.
func (r *QueriesRepository) Create(ctx context.Context, userID int64, name, query string) (SavedSearch, error) {
	row, err := r.q.CreateSavedSearch(ctx, db.CreateSavedSearchParams{UserID: userID, Name: name, Query: query})
	if pgerr.IsUniqueViolation(err) {
		return SavedSearch{}, ErrDuplicateName
	}
	if err != nil {
		return SavedSearch{}, err
	}
	return fromRow(row), nil
}

// Update overwrites a saved search scoped to its owner. A nil name/query is left unchanged
// (NULL param). No matching row (wrong id or another user's) returns no row → ErrNotFound;
// a name collision → ErrDuplicateName.
func (r *QueriesRepository) Update(ctx context.Context, id, userID int64, name, query *string) (SavedSearch, error) {
	row, err := r.q.UpdateSavedSearch(ctx, db.UpdateSavedSearchParams{
		ID:     id,
		UserID: userID,
		Name:   textPtr(name),
		Query:  textPtr(query),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SavedSearch{}, ErrNotFound
	}
	if pgerr.IsUniqueViolation(err) {
		return SavedSearch{}, ErrDuplicateName
	}
	if err != nil {
		return SavedSearch{}, err
	}
	return fromRow(row), nil
}

// Delete removes a saved search scoped to its owner, mapping "no row affected" (missing
// or non-owned) to ErrNotFound.
func (r *QueriesRepository) Delete(ctx context.Context, id, userID int64) error {
	affected, err := r.q.DeleteSavedSearch(ctx, db.DeleteSavedSearchParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Get reads one of a user's saved searches, owner-scoped, mapping "no row" (missing or
// another user's) to ErrNotFound.
func (r *QueriesRepository) Get(ctx context.Context, id, userID int64) (SavedSearch, error) {
	row, err := r.q.GetSavedSearch(ctx, db.GetSavedSearchParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SavedSearch{}, ErrNotFound
	}
	if err != nil {
		return SavedSearch{}, err
	}
	return fromRow(row), nil
}

// SetPublicSlug publishes a board scoped to its owner, mapping a slug UNIQUE collision to
// ErrSlugTaken (retried by the service) and "no row" (missing or non-owned) to ErrNotFound.
// An empty authorLabel is stored NULL (anonymous).
func (r *QueriesRepository) SetPublicSlug(ctx context.Context, id, userID int64, publicSlug, authorLabel string) (SavedSearch, error) {
	row, err := r.q.SetSavedSearchPublicSlug(ctx, db.SetSavedSearchPublicSlugParams{
		ID:          id,
		UserID:      userID,
		PublicSlug:  pgtype.Text{String: publicSlug, Valid: true},
		AuthorLabel: text(authorLabel),
	})
	if pgerr.IsUniqueViolation(err) {
		return SavedSearch{}, ErrSlugTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return SavedSearch{}, ErrNotFound
	}
	if err != nil {
		return SavedSearch{}, err
	}
	return fromRow(row), nil
}

// ClearPublicSlug unpublishes a board scoped to its owner, mapping "no row affected"
// (missing or non-owned) to ErrNotFound. Clearing an already-private owned row still
// matches (row count 1), so unshare is an idempotent no-op.
func (r *QueriesRepository) ClearPublicSlug(ctx context.Context, id, userID int64) error {
	affected, err := r.q.ClearSavedSearchPublicSlug(ctx, db.ClearSavedSearchPublicSlugParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPublicBoard reads a shared board by slug (no auth, no owner-scoping), mapping "no row"
// (unknown or unshared slug) to ErrNotFound.
func (r *QueriesRepository) GetPublicBoard(ctx context.Context, slug string) (Board, error) {
	row, err := r.q.GetPublicBoardBySlug(ctx, pgtype.Text{String: slug, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return Board{}, ErrNotFound
	}
	if err != nil {
		return Board{}, err
	}
	return Board{Name: row.Name, Query: row.Query, AuthorLabel: row.AuthorLabel.String}, nil
}

// fromRow maps the generated db row to the package domain type, collapsing the nullable
// public_slug/author_label columns to plain strings (NULL → "") and dropping the owner
// column the use case does not need.
func fromRow(row db.SavedSearch) SavedSearch {
	return SavedSearch{
		ID:          row.ID,
		Name:        row.Name,
		Query:       row.Query,
		PublicSlug:  row.PublicSlug.String,
		AuthorLabel: row.AuthorLabel.String,
		CreatedAt:   pgconv.TimePtr(row.CreatedAt),
		UpdatedAt:   pgconv.TimePtr(row.UpdatedAt),
	}
}

// text maps a string to the pgtype the generated params expect: empty becomes the zero
// (NULL) value, a non-empty string a valid text.
func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// textPtr maps an optional string to the pgtype a partial update expects: nil becomes the
// zero (NULL, "leave unchanged") value, a non-nil pointer a valid text (an empty string is
// a real "show all" query value, so it stays valid).
func textPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
