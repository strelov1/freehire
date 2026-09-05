package savedsearch

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
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

// Create inserts a saved search. The UNIQUE (user_id, name) violation maps to
// ErrDuplicateName; the partial UNIQUE (user_id) WHERE derived_from_profile violation
// (at most one profile-derived row per user) maps to ErrProfileSearchExists.
func (r *QueriesRepository) Create(ctx context.Context, userID int64, name, query string, derivedFromProfile bool) (SavedSearch, error) {
	row, err := r.q.CreateSavedSearch(ctx, db.CreateSavedSearchParams{
		UserID:             userID,
		Name:               name,
		Query:              query,
		DerivedFromProfile: derivedFromProfile,
	})
	if constraint, ok := pgerr.UniqueViolationConstraint(err); ok {
		if constraint == "saved_searches_derived_from_profile_idx" {
			return SavedSearch{}, ErrProfileSearchExists
		}
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

// fromRow maps the generated db row to the package domain type, dropping the owner
// column the use case does not need.
func fromRow(row db.SavedSearch) SavedSearch {
	return SavedSearch{
		ID:                 row.ID,
		Name:               row.Name,
		Query:              row.Query,
		DerivedFromProfile: row.DerivedFromProfile,
		CreatedAt:          pgconv.TimePtr(row.CreatedAt),
		UpdatedAt:          pgconv.TimePtr(row.UpdatedAt),
	}
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
