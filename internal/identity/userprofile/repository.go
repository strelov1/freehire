package userprofile

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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
func (r *QueriesRepository) Get(ctx context.Context, userID int64) (Profile, error) {
	row, err := r.q.GetUserProfile(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// Upsert creates or replaces the user's profile.
func (r *QueriesRepository) Upsert(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, locationPreferences json.RawMessage) (Profile, error) {
	row, err := r.q.UpsertUserProfile(ctx, db.UpsertUserProfileParams{
		UserID:              userID,
		Specializations:     specializations,
		Skills:              skills,
		Seniorities:         seniorities,
		ExcludedSkills:      excludedSkills,
		LocationPreferences: locationPreferences,
	})
	if err != nil {
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// UpsertIfUnchanged behaves like Upsert but only writes when the row's updated_at still
// matches expectedUpdatedAt, mapping no matching row to ErrConflict.
func (r *QueriesRepository) UpsertIfUnchanged(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, locationPreferences json.RawMessage, expectedUpdatedAt time.Time) (Profile, error) {
	row, err := r.q.UpsertUserProfileIfUnchanged(ctx, db.UpsertUserProfileIfUnchangedParams{
		UserID:              userID,
		Specializations:     specializations,
		Skills:              skills,
		Seniorities:         seniorities,
		ExcludedSkills:      excludedSkills,
		LocationPreferences: locationPreferences,
		UpdatedAt:           pgtype.Timestamptz{Time: expectedUpdatedAt, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrConflict
	}
	if err != nil {
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// profileFromRow maps the generated db row to the package domain type, dropping the
// internal bookkeeping columns the use case does not need.
func profileFromRow(row db.UserProfile) Profile {
	return Profile{
		UserID:              row.UserID,
		Specializations:     row.Specializations,
		Skills:              row.Skills,
		Seniorities:         row.Seniorities,
		ExcludedSkills:      row.ExcludedSkills,
		LocationPreferences: row.LocationPreferences,
		CreatedAt:           pgconv.TimePtr(row.CreatedAt),
		UpdatedAt:           pgconv.TimePtr(row.UpdatedAt),
	}
}

// Delete removes the user's profile. The affected-row count is ignored: deleting when
// none exists is not an error (idempotent).
func (r *QueriesRepository) Delete(ctx context.Context, userID int64) error {
	_, err := r.q.DeleteUserProfile(ctx, userID)
	return err
}
