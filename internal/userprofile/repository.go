package userprofile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository. It maps the no-row condition on
// Get to ErrNotFound; Upsert and Delete need no mapping (the PRIMARY KEY (user_id) makes
// Upsert conflict-free and Delete is idempotent).
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Get returns the user's profile, mapping no row to ErrNotFound.
func (r *QueriesRepository) Get(ctx context.Context, userID int64) (db.UserProfile, error) {
	row, err := r.q.GetUserProfile(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.UserProfile{}, ErrNotFound
	}
	if err != nil {
		return db.UserProfile{}, err
	}
	return row, nil
}

// Upsert creates or replaces the user's profile.
func (r *QueriesRepository) Upsert(ctx context.Context, p db.UpsertUserProfileParams) (db.UserProfile, error) {
	return r.q.UpsertUserProfile(ctx, p)
}

// Delete removes the user's profile. The affected-row count is ignored: deleting when
// none exists is not an error (idempotent).
func (r *QueriesRepository) Delete(ctx context.Context, userID int64) error {
	_, err := r.q.DeleteUserProfile(ctx, userID)
	return err
}
