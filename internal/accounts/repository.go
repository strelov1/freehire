package accounts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// QueriesRepository is the production Repository backed by sqlc-generated
// *db.Queries and a *pgxpool.Pool for transaction management.
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
}

// Compile-time assertion that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// UserIDByIdentity returns the local user id for an external identity, or
// ErrIdentityNotFound when no identity row matches.
func (r *QueriesRepository) UserIDByIdentity(ctx context.Context, provider, providerUserID string) (int64, error) {
	row, err := r.q.GetUserByIdentity(ctx, db.GetUserByIdentityParams{
		Provider:       provider,
		ProviderUserID: providerUserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrIdentityNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// LinkOrCreateByEmail links the given identity to the account with the
// supplied (already-lowercased) email, creating a passwordless account when
// none exists. The operation runs in a single transaction. A concurrent-callback
// race surfaces as one of two distinct errors: ErrEmailRace when the account was
// created first under the same email (our CreateUser lost the email index, so our
// identity was never inserted), or ErrIdentityConflict when our exact identity was
// inserted first. The service recovers from each differently.
func (r *QueriesRepository) LinkOrCreateByEmail(
	ctx context.Context,
	provider, providerUserID, email string,
) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)

	var userID int64

	existing, err := q.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		userID = existing.ID
	case errors.Is(err, pgx.ErrNoRows):
		created, err := q.CreateUser(ctx, db.CreateUserParams{
			Email:        email,
			PasswordHash: pgtype.Text{}, // passwordless OAuth account
		})
		if err != nil {
			if pgerr.IsUniqueViolation(err) {
				// users has a single unique index (lower(email)), so this can only be
				// a concurrent account creation for the same email — not our identity.
				return 0, ErrEmailRace
			}
			return 0, err
		}
		userID = created.ID
	default:
		return 0, err
	}

	if err := q.CreateUserIdentity(ctx, db.CreateUserIdentityParams{
		Provider:       provider,
		ProviderUserID: providerUserID,
		UserID:         userID,
	}); err != nil {
		if pgerr.IsUniqueViolation(err) {
			return 0, ErrIdentityConflict
		}
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		if pgerr.IsUniqueViolation(err) {
			return 0, ErrIdentityConflict
		}
		return 0, err
	}

	return userID, nil
}

// CreateUser inserts a new account and returns it. Returns ErrEmailTaken on a
// unique-constraint violation.
func (r *QueriesRepository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	row, err := r.q.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
	})
	if pgerr.IsUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester, CreatedAt: pgconv.TimePtr(row.CreatedAt)}, nil
}

// UserByEmail looks up the account with the given (already-normalised) email.
// Returns ErrUserNotFound when absent. hasPassword is true when a non-null
// password hash is stored.
func (r *QueriesRepository) UserByEmail(ctx context.Context, email string) (User, string, bool, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", false, ErrUserNotFound
	}
	if err != nil {
		return User{}, "", false, err
	}
	u := User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester, CreatedAt: pgconv.TimePtr(row.CreatedAt)}
	return u, row.PasswordHash.String, row.PasswordHash.Valid, nil
}

// UserByID returns the user with the given id, or ErrUserNotFound when absent.
func (r *QueriesRepository) UserByID(ctx context.Context, id int64) (User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester, Points: int(row.Points), CreatedAt: pgconv.TimePtr(row.CreatedAt)}, nil
}
