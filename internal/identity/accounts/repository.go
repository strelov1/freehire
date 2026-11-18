package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// QueriesRepository is the production Repository, backed by sqlc queries and a pool.
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
}

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
	// Status is part of authentication: an Apple identity awaiting provider revocation
	// must not sign in and undo the unlink.
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

// LinkOrCreateByEmail links the identity to the account with this lowercased email,
// creating a passwordless account when none exists, in one transaction. A concurrent
// race surfaces as ErrEmailRace (account created first, our identity not inserted) or
// ErrIdentityConflict (our identity inserted first).
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
	createdAccount := false

	existing, err := q.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		userID = existing.ID
		// Account pre-hijacking defence: a provider vouched for this address, but a squatter
		// may have registered it unverified. An unverified, password-backed account is
		// SEIZED (password destroyed, sessions revoked) in this transaction. A verified one
		// is joined; a passwordless one is only marked verified.
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
			// Born verified: ResolveOAuthAccount only gets here with a provider-verified email.
			EmailVerified: true,
		})
		if err != nil {
			if pgerr.IsUniqueViolation(err) {
				// users has one unique index (lower(email)), so this is a concurrent
				// creation for the same email, not our identity.
				return 0, ErrEmailRace
			}
			return 0, err
		}
		userID = created.ID
		createdAccount = true
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

	if createdAccount {
		observability.RecordRegistration()
	}
	return userID, nil
}

// CreateUser inserts an account. Returns ErrEmailTaken on a unique violation.
func (r *QueriesRepository) CreateUser(ctx context.Context, email, passwordHash string, emailVerified bool, timezone *string) (User, error) {
	tz := ""
	if timezone != nil {
		tz = *timezone
	}
	row, err := r.q.CreateUser(ctx, db.CreateUserParams{
		Email:         email,
		PasswordHash:  pgconv.TextValid(passwordHash),
		EmailVerified: emailVerified,
		Timezone:      pgconv.Text(tz),
	})
	if pgerr.IsUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, err
	}
	observability.RecordRegistration()
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, HasPassword: true,
		CreatedAt: pgconv.TimePtr(row.CreatedAt), Timezone: pgconv.TextPtr(row.Timezone),
		Language: row.Language}, nil
}

// UpdateTimezone sets the account's IANA timezone; the caller validated it.
func (r *QueriesRepository) UpdateTimezone(ctx context.Context, userID int64, timezone *string, onlyIfUnset bool) (User, error) {
	tz := ""
	if timezone != nil {
		tz = *timezone
	}
	row, err := r.q.UpdateUserTimezone(ctx, db.UpdateUserTimezoneParams{ID: userID, Timezone: pgconv.Text(tz), OnlyIfUnset: onlyIfUnset})
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, HasPassword: row.HasPassword,
		CreatedAt: pgconv.TimePtr(row.CreatedAt), Timezone: pgconv.TextPtr(row.Timezone),
		Language: row.Language}, nil
}

// UpdateLanguage sets the preferred language; the caller validated it.
func (r *QueriesRepository) UpdateLanguage(ctx context.Context, userID int64, language string) (User, error) {
	row, err := r.q.UpdateUserLanguage(ctx, db.UpdateUserLanguageParams{ID: userID, Language: language})
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, HasPassword: row.HasPassword,
		CreatedAt: pgconv.TimePtr(row.CreatedAt), Timezone: pgconv.TextPtr(row.Timezone),
		Language: row.Language}, nil
}

// MarkEmailVerified records that control of the address was proven. Idempotent.
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
		PasswordHash: pgconv.TextValid(passwordHash),
	})
}

// ResetPassword sets a password after a mailed code proved the address, marking the
// account verified and revoking every session. Returns the new session generation.
func (r *QueriesRepository) ResetPassword(ctx context.Context, userID int64, passwordHash string) (int32, error) {
	return r.q.ResetUserPassword(ctx, db.ResetUserPasswordParams{
		ID:           userID,
		PasswordHash: pgconv.TextValid(passwordHash),
	})
}

// ChangeEmail moves the account to a new, code-proven address: verified, every session
// revoked, and the codes mailed to the old address destroyed. Returns ErrEmailTaken when
// another account claimed the address since the code was issued.
func (r *QueriesRepository) ChangeEmail(ctx context.Context, userID int64, email string) (int32, error) {
	version, err := r.q.ChangeUserEmail(ctx, db.ChangeUserEmailParams{ID: userID, Email: email})
	if pgerr.IsUniqueViolation(err) {
		return 0, ErrEmailTaken
	}
	return version, err
}

// DeleteOAuthGrantsByUser wipes every MCP OAuth grant the account holds.
func (r *QueriesRepository) DeleteOAuthGrantsByUser(ctx context.Context, userID int64) error {
	return r.q.DeleteOAuthGrantsByUser(ctx, userID)
}

// UserByEmail looks up an account by normalised email, or ErrUserNotFound.
func (r *QueriesRepository) UserByEmail(ctx context.Context, email string) (User, string, bool, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", false, ErrUserNotFound
	}
	if err != nil {
		return User{}, "", false, err
	}
	u := User{ID: row.ID, Email: row.Email, Role: row.Role, BetaTester: row.BetaTester,
		EmailVerified: row.EmailVerified, CreatedAt: pgconv.TimePtr(row.CreatedAt),
		Language: row.Language, Banned: row.Banned}
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
		CreatedAt: pgconv.TimePtr(row.CreatedAt), Timezone: pgconv.TextPtr(row.Timezone),
		Language: row.Language, OnboardingCompletedAt: pgconv.TimePtr(row.OnboardingCompletedAt)}, nil
}

// UsernameByUser returns the account's username (ok=false if none) and, when
// set, the time of its last explicit change.
func (r *QueriesRepository) UsernameByUser(ctx context.Context, userID int64) (string, *time.Time, bool, error) {
	row, err := r.q.GetUsernameByUser(ctx, userID)
	if err != nil {
		return "", nil, false, err
	}
	if !row.Username.Valid {
		return "", nil, false, nil
	}
	return row.Username.String, pgconv.TimePtr(row.UsernameUpdatedAt), true, nil
}

// SetUsernameIfAbsent claims username only if the account has none. A unique violation
// and zero rows affected both map to ErrUsernameTaken.
func (r *QueriesRepository) SetUsernameIfAbsent(ctx context.Context, userID int64, username string) error {
	n, err := r.q.SetUsernameIfAbsent(ctx, db.SetUsernameIfAbsentParams{ID: userID, Username: pgconv.Text(username)})
	if pgerr.IsUniqueViolation(err) {
		return ErrUsernameTaken
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUsernameTaken
	}
	return nil
}

// SetUsername replaces the account's username unconditionally. Maps a unique
// violation to ErrUsernameTaken.
func (r *QueriesRepository) SetUsername(ctx context.Context, userID int64, username string) error {
	err := r.q.SetUsername(ctx, db.SetUsernameParams{ID: userID, Username: pgconv.Text(username)})
	if pgerr.IsUniqueViolation(err) {
		return ErrUsernameTaken
	}
	return err
}

// UsernameTaken reports whether any account already holds username.
func (r *QueriesRepository) UsernameTaken(ctx context.Context, username string) (bool, error) {
	_, err := r.q.GetUserIDByUsername(ctx, pgconv.Text(username))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
