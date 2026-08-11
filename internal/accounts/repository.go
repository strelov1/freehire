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

// WithTx returns a transactional copy of QueriesRepository.
func (r *QueriesRepository) WithTx(tx pgx.Tx) Repository {
	if tx == nil {
		return r
	}
	return &QueriesRepository{
		q:    r.q.WithTx(tx),
		pool: r.pool,
	}
}

// UserIDByIdentity returns the local user id for an external identity, or
// ErrIdentityNotFound when no identity row matches.
func (r *QueriesRepository) UserIDByIdentity(ctx context.Context, provider, providerUserID string) (int64, error) {
	// Status is part of authentication, not display metadata: an Apple identity
	// awaiting provider revocation must not sign in and silently undo unlink.
	row, err := r.q.GetActiveUserByIdentity(ctx, db.GetActiveUserByIdentityParams{
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
		// Account pre-hijacking defence. Reaching here means a provider has vouched for
		// this address, but the account it matches may have been registered by someone
		// who never proved they own it — the squatter would keep password access to
		// everything the real owner does from now on. So an unverified, password-backed
		// account is not joined, it is SEIZED: the password is destroyed and every
		// session revoked, in this same transaction. A verified account is joined as
		// before (its owner already proved the address), and a passwordless one has
		// nothing to seize — it is only marked verified.
		if !existing.EmailVerified {
			if existing.PasswordHash.Valid {
				if _, err := q.SeizeUnverifiedAccount(ctx, userID); err != nil {
					return 0, err
				}
			} else if err := q.SetUserEmailVerified(ctx, userID); err != nil {
				return 0, err
			}
		}
	case errors.Is(err, pgx.ErrNoRows):
		created, err := q.CreateUser(ctx, db.CreateUserParams{
			Email:        email,
			PasswordHash: pgtype.Text{}, // passwordless OAuth account
			// Born verified: the caller only reaches this path with a provider-verified
			// email (ResolveOAuthAccount's gate), so ownership is already proven.
			EmailVerified: true,
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
func (r *QueriesRepository) CreateUser(ctx context.Context, email, passwordHash string, emailVerified bool) (User, error) {
	row, err := r.q.CreateUser(ctx, db.CreateUserParams{
		Email:         email,
		PasswordHash:  pgtype.Text{String: passwordHash, Valid: true},
		EmailVerified: emailVerified,
	})
	if pgerr.IsUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, HasPassword: true,
		CreatedAt: pgconv.TimePtr(row.CreatedAt)}, nil
}

// MarkEmailVerified records that control of the account's address was proven, by an
// emailed code or by a provider. Idempotent.
func (r *QueriesRepository) MarkEmailVerified(ctx context.Context, userID int64) error {
	return r.q.SetUserEmailVerified(ctx, userID)
}

// PasswordHash returns the account's stored bcrypt hash; hasPassword is false for a
// passwordless (OAuth-only) account and for one that no longer exists.
func (r *QueriesRepository) PasswordHash(ctx context.Context, userID int64) (string, bool, error) {
	hash, err := r.q.GetUserPasswordHash(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrUserNotFound
	}
	if err != nil {
		return "", false, err
	}
	return hash.String, hash.Valid, nil
}

// SetPassword replaces a known password, revoking every session in the same statement.
// Returns the account's new session generation.
func (r *QueriesRepository) SetPassword(ctx context.Context, userID int64, passwordHash string) (int32, error) {
	return r.q.SetUserPassword(ctx, db.SetUserPasswordParams{
		ID:           userID,
		PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
	})
}

// ResetPassword sets a password after a mailed code proved the address, marking the
// account verified and revoking every session. Returns the new session generation.
func (r *QueriesRepository) ResetPassword(ctx context.Context, userID int64, passwordHash string) (int32, error) {
	return r.q.ResetUserPassword(ctx, db.ResetUserPasswordParams{
		ID:           userID,
		PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
	})
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
	u := User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, CreatedAt: pgconv.TimePtr(row.CreatedAt)}
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
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, HasPassword: row.HasPassword,
		CreatedAt: pgconv.TimePtr(row.CreatedAt)}, nil
}
