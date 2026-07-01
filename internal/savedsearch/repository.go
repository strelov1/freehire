package savedsearch

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
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
func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]db.SavedSearch, error) {
	return r.q.ListSavedSearches(ctx, userID)
}

// Count returns how many saved searches the user has (the cap check input).
func (r *QueriesRepository) Count(ctx context.Context, userID int64) (int64, error) {
	return r.q.CountSavedSearches(ctx, userID)
}

// Create inserts a saved search, mapping the UNIQUE (user_id, name) violation to
// ErrDuplicateName.
func (r *QueriesRepository) Create(ctx context.Context, p db.CreateSavedSearchParams) (db.SavedSearch, error) {
	row, err := r.q.CreateSavedSearch(ctx, p)
	if isUniqueViolation(err) {
		return db.SavedSearch{}, ErrDuplicateName
	}
	if err != nil {
		return db.SavedSearch{}, err
	}
	return row, nil
}

// Update overwrites a saved search scoped to its owner. No matching row (wrong id or
// another user's) returns no row → ErrNotFound; a name collision → ErrDuplicateName.
func (r *QueriesRepository) Update(ctx context.Context, p db.UpdateSavedSearchParams) (db.SavedSearch, error) {
	row, err := r.q.UpdateSavedSearch(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SavedSearch{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return db.SavedSearch{}, ErrDuplicateName
	}
	if err != nil {
		return db.SavedSearch{}, err
	}
	return row, nil
}

// Delete removes a saved search scoped to its owner, mapping "no row affected" (missing
// or non-owned) to ErrNotFound.
func (r *QueriesRepository) Delete(ctx context.Context, p db.DeleteSavedSearchParams) error {
	affected, err := r.q.DeleteSavedSearch(ctx, p)
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
func (r *QueriesRepository) Get(ctx context.Context, p db.GetSavedSearchParams) (db.SavedSearch, error) {
	row, err := r.q.GetSavedSearch(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SavedSearch{}, ErrNotFound
	}
	if err != nil {
		return db.SavedSearch{}, err
	}
	return row, nil
}

// SetPublicSlug publishes a board scoped to its owner, mapping a slug UNIQUE collision to
// ErrSlugTaken (retried by the service) and "no row" (missing or non-owned) to ErrNotFound.
func (r *QueriesRepository) SetPublicSlug(ctx context.Context, p db.SetSavedSearchPublicSlugParams) (db.SavedSearch, error) {
	row, err := r.q.SetSavedSearchPublicSlug(ctx, p)
	if isUniqueViolation(err) {
		return db.SavedSearch{}, ErrSlugTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SavedSearch{}, ErrNotFound
	}
	if err != nil {
		return db.SavedSearch{}, err
	}
	return row, nil
}

// ClearPublicSlug unpublishes a board scoped to its owner, mapping "no row affected"
// (missing or non-owned) to ErrNotFound. Clearing an already-private owned row still
// matches (row count 1), so unshare is an idempotent no-op.
func (r *QueriesRepository) ClearPublicSlug(ctx context.Context, p db.ClearSavedSearchPublicSlugParams) error {
	affected, err := r.q.ClearSavedSearchPublicSlug(ctx, p)
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
func (r *QueriesRepository) GetPublicBoard(ctx context.Context, slug string) (db.GetPublicBoardBySlugRow, error) {
	row, err := r.q.GetPublicBoardBySlug(ctx, pgtype.Text{String: slug, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetPublicBoardBySlugRow{}, ErrNotFound
	}
	if err != nil {
		return db.GetPublicBoardBySlugRow{}, err
	}
	return row, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
