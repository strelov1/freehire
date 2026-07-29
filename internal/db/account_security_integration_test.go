//go:build integration

// Integration tests for the account-security query semantics — the one-code-per-purpose
// constraint on user_email_codes, the token_version counter, and the API-key scope — which
// are SQL behavior (upsert, RETURNING, defaults) and can only be verified against a real
// Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedAccount creates a password-backed, unverified account the way registration does.
func seedAccount(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash) VALUES ($1, 'hash') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func TestUserEmailCodeQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAccount(t, pool, "codes@example.test")
	future := pgtype.Timestamptz{Time: time.Now().Add(15 * time.Minute), Valid: true}

	t.Run("upsert replaces the outstanding code and resets attempts", func(t *testing.T) {
		if err := q.UpsertEmailCode(ctx, UpsertEmailCodeParams{
			UserID: user, Purpose: "verify_email", CodeHash: "first", ExpiresAt: future,
		}); err != nil {
			t.Fatalf("UpsertEmailCode(first): %v", err)
		}
		if _, err := q.BumpEmailCodeAttempts(ctx, BumpEmailCodeAttemptsParams{
			UserID: user, Purpose: "verify_email",
		}); err != nil {
			t.Fatalf("BumpEmailCodeAttempts: %v", err)
		}

		if err := q.UpsertEmailCode(ctx, UpsertEmailCodeParams{
			UserID: user, Purpose: "verify_email", CodeHash: "second", ExpiresAt: future,
		}); err != nil {
			t.Fatalf("UpsertEmailCode(second): %v", err)
		}

		got, err := q.GetEmailCode(ctx, GetEmailCodeParams{UserID: user, Purpose: "verify_email"})
		if err != nil {
			t.Fatalf("GetEmailCode: %v", err)
		}
		if got.CodeHash != "second" {
			t.Errorf("code_hash = %q, want the replacement %q", got.CodeHash, "second")
		}
		if got.Attempts != 0 {
			t.Errorf("attempts = %d, want 0 after a resend", got.Attempts)
		}

		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM user_email_codes WHERE user_id = $1 AND purpose = 'verify_email'`,
			user).Scan(&rows); err != nil {
			t.Fatalf("count codes: %v", err)
		}
		if rows != 1 {
			t.Errorf("outstanding codes = %d, want exactly 1", rows)
		}
	})

	t.Run("the two purposes are independent", func(t *testing.T) {
		if err := q.UpsertEmailCode(ctx, UpsertEmailCodeParams{
			UserID: user, Purpose: "password_reset", CodeHash: "reset", ExpiresAt: future,
		}); err != nil {
			t.Fatalf("UpsertEmailCode(password_reset): %v", err)
		}
		verify, err := q.GetEmailCode(ctx, GetEmailCodeParams{UserID: user, Purpose: "verify_email"})
		if err != nil {
			t.Fatalf("GetEmailCode(verify_email): %v", err)
		}
		if verify.CodeHash != "second" {
			t.Errorf("a password_reset code overwrote the verify_email one (%q)", verify.CodeHash)
		}
	})

	t.Run("attempts increment and are returned", func(t *testing.T) {
		n, err := q.BumpEmailCodeAttempts(ctx, BumpEmailCodeAttemptsParams{
			UserID: user, Purpose: "password_reset",
		})
		if err != nil {
			t.Fatalf("BumpEmailCodeAttempts: %v", err)
		}
		if n != 1 {
			t.Errorf("attempts = %d, want 1", n)
		}
	})

	t.Run("delete consumes the code", func(t *testing.T) {
		if err := q.DeleteEmailCode(ctx, DeleteEmailCodeParams{
			UserID: user, Purpose: "password_reset",
		}); err != nil {
			t.Fatalf("DeleteEmailCode: %v", err)
		}
		if _, err := q.GetEmailCode(ctx, GetEmailCodeParams{
			UserID: user, Purpose: "password_reset",
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("err = %v, want pgx.ErrNoRows after the code is consumed", err)
		}
	})

	t.Run("codes die with their account", func(t *testing.T) {
		doomed := seedAccount(t, pool, "doomed@example.test")
		if err := q.UpsertEmailCode(ctx, UpsertEmailCodeParams{
			UserID: doomed, Purpose: "verify_email", CodeHash: "x", ExpiresAt: future,
		}); err != nil {
			t.Fatalf("UpsertEmailCode: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, doomed); err != nil {
			t.Fatalf("delete user: %v", err)
		}
		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM user_email_codes WHERE user_id = $1`, doomed).Scan(&rows); err != nil {
			t.Fatalf("count codes: %v", err)
		}
		if rows != 0 {
			t.Errorf("orphan codes = %d, want 0 (cascade)", rows)
		}
	})
}

func TestAccountVerificationAndTokenVersion(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a registered account starts unverified at version 1", func(t *testing.T) {
		created, err := q.CreateUser(ctx, CreateUserParams{
			Email:        "fresh@example.test",
			PasswordHash: pgtype.Text{String: "hash", Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if created.EmailVerified {
			t.Error("a password registration must start unverified")
		}
		v, err := q.GetUserTokenVersion(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetUserTokenVersion: %v", err)
		}
		if v != 1 {
			t.Errorf("token_version = %d, want 1 so a claimless legacy token never matches", v)
		}
	})

	t.Run("an OAuth account is created verified", func(t *testing.T) {
		created, err := q.CreateUser(ctx, CreateUserParams{
			Email:         "oauth@example.test",
			EmailVerified: true,
		})
		if err != nil {
			t.Fatalf("CreateUser(verified): %v", err)
		}
		if !created.EmailVerified {
			t.Error("an OAuth-created account must be verified at creation")
		}
	})

	t.Run("confirming a code marks the account verified", func(t *testing.T) {
		user := seedAccount(t, pool, "confirm@example.test")
		if err := q.SetUserEmailVerified(ctx, user); err != nil {
			t.Fatalf("SetUserEmailVerified: %v", err)
		}
		got, err := q.GetUserByID(ctx, user)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}
		if !got.EmailVerified {
			t.Error("account is still unverified after SetUserEmailVerified")
		}
	})

	t.Run("bumping the token version revokes older tokens", func(t *testing.T) {
		user := seedAccount(t, pool, "revoke@example.test")
		next, err := q.BumpUserTokenVersion(ctx, user)
		if err != nil {
			t.Fatalf("BumpUserTokenVersion: %v", err)
		}
		if next != 2 {
			t.Errorf("token_version = %d, want 2", next)
		}
	})

	t.Run("a password reset re-verifies the email and revokes sessions", func(t *testing.T) {
		user := seedAccount(t, pool, "reset@example.test")
		version, err := q.ResetUserPassword(ctx, ResetUserPasswordParams{
			ID: user, PasswordHash: pgtype.Text{String: "new-hash", Valid: true},
		})
		if err != nil {
			t.Fatalf("ResetUserPassword: %v", err)
		}
		if version != 2 {
			t.Errorf("token_version = %d, want 2 (reset revokes sessions)", version)
		}
		row, _, hasPassword, err := getUserByEmailFields(ctx, q, "reset@example.test")
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}
		if !hasPassword {
			t.Error("the account lost its password on reset")
		}
		if !row.EmailVerified {
			t.Error("a completed reset proves email ownership and must verify the account")
		}
	})

	t.Run("changing a password revokes sessions without touching verification", func(t *testing.T) {
		user := seedAccount(t, pool, "change@example.test")
		version, err := q.SetUserPassword(ctx, SetUserPasswordParams{
			ID: user, PasswordHash: pgtype.Text{String: "changed", Valid: true},
		})
		if err != nil {
			t.Fatalf("SetUserPassword: %v", err)
		}
		if version != 2 {
			t.Errorf("token_version = %d, want 2 (a change revokes other sessions)", version)
		}
		got, err := q.GetUserByID(ctx, user)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}
		if got.EmailVerified {
			t.Error("changing a password must not verify the email")
		}
	})

	t.Run("seizing an unverified account kills its password and sessions", func(t *testing.T) {
		user := seedAccount(t, pool, "seized@example.test")
		version, err := q.SeizeUnverifiedAccount(ctx, user)
		if err != nil {
			t.Fatalf("SeizeUnverifiedAccount: %v", err)
		}
		if version != 2 {
			t.Errorf("token_version = %d, want 2 (the squatter's sessions are revoked)", version)
		}
		row, _, hasPassword, err := getUserByEmailFields(ctx, q, "seized@example.test")
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}
		if hasPassword {
			t.Error("the seized account kept its password hash")
		}
		if !row.EmailVerified {
			t.Error("the seized account must end up verified — the provider proved the address")
		}
	})
}

func TestAPIKeyScope(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	// Verified: CreateAPIKey refuses an account whose address was never proven, so a
	// scope test has to start from an account that is allowed to hold keys at all.
	user := seedAccount(t, pool, "scoped@example.test")
	if _, err := pool.Exec(ctx, `UPDATE users SET email_verified = true WHERE id = $1`, user); err != nil {
		t.Fatalf("verify account: %v", err)
	}

	t.Run("authentication reports the key's scope", func(t *testing.T) {
		if _, err := q.CreateAPIKey(ctx, CreateAPIKeyParams{
			UserID: user, Name: "cli", TokenHash: "hash-full", TokenPrefix: "fhk_full01", Scope: "full",
		}); err != nil {
			t.Fatalf("CreateAPIKey(full): %v", err)
		}
		if _, err := q.CreateAPIKey(ctx, CreateAPIKeyParams{
			UserID: user, Name: "tailoring", TokenHash: "hash-cv", TokenPrefix: "fhk_cv0001", Scope: "cv",
		}); err != nil {
			t.Fatalf("CreateAPIKey(cv): %v", err)
		}

		full, err := q.AuthenticateAPIKey(ctx, "hash-full")
		if err != nil {
			t.Fatalf("AuthenticateAPIKey(full): %v", err)
		}
		if full.UserID != user || full.Scope != "full" {
			t.Errorf("full key resolved to (%d, %q), want (%d, \"full\")", full.UserID, full.Scope, user)
		}

		narrow, err := q.AuthenticateAPIKey(ctx, "hash-cv")
		if err != nil {
			t.Fatalf("AuthenticateAPIKey(cv): %v", err)
		}
		if narrow.Scope != "cv" {
			t.Errorf("narrow key scope = %q, want \"cv\"", narrow.Scope)
		}
	})

	t.Run("an unknown scope is refused by the database", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO api_keys (user_id, name, token_hash, token_prefix, scope)
			 VALUES ($1, 'bogus', 'hash-bogus', 'fhk_bogus1', 'admin')`, user); err == nil {
			t.Error("the database accepted an unknown scope; the CHECK constraint is missing")
		}
	})

	t.Run("existing keys default to full", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO api_keys (user_id, name, token_hash, token_prefix)
			 VALUES ($1, 'legacy', 'hash-legacy', 'fhk_leg001')`, user); err != nil {
			t.Fatalf("insert legacy key: %v", err)
		}
		got, err := q.AuthenticateAPIKey(ctx, "hash-legacy")
		if err != nil {
			t.Fatalf("AuthenticateAPIKey(legacy): %v", err)
		}
		if got.Scope != "full" {
			t.Errorf("legacy key scope = %q, want \"full\" so current integrations are unaffected", got.Scope)
		}
	})

	t.Run("the listing exposes the scope", func(t *testing.T) {
		keys, err := q.ListAPIKeysByUser(ctx, user)
		if err != nil {
			t.Fatalf("ListAPIKeysByUser: %v", err)
		}
		var seenCV bool
		for _, k := range keys {
			if k.Scope == "cv" {
				seenCV = true
			}
		}
		if !seenCV {
			t.Error("the key listing does not report scopes")
		}
	})
}

// getUserByEmailFields adapts GetUserByEmail's row to the (row, hash, hasPassword) shape the
// assertions read, keeping the tests readable as the generated row grows columns.
func getUserByEmailFields(ctx context.Context, q *Queries, email string) (GetUserByEmailRow, string, bool, error) {
	row, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		return GetUserByEmailRow{}, "", false, err
	}
	return row, row.PasswordHash.String, row.PasswordHash.Valid, nil
}
