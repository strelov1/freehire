//go:build integration

// Integration tests for the api_keys query semantics — owner resolution with the
// last_used_at touch, expiry enforcement, and owner-scoped delete — which are SQL
// behavior and can only be verified against a real Postgres. Run with:
// go test -tags=integration ./internal/db/
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

// seedAPIKeyUser creates an account that may hold API keys — verified, because
// CreateAPIKey refuses an address that was never proven.
func seedAPIKeyUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func TestAPIKeyQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedAPIKeyUser(t, pool, "alice@example.test")
	bob := seedAPIKeyUser(t, pool, "bob@example.test")

	const liveHash = "hash-alice-live"
	live, err := q.CreateAPIKey(ctx, CreateAPIKeyParams{
		UserID: alice, Name: "ci", TokenHash: liveHash, TokenPrefix: "fhk_alice1", Scope: "full",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if live.LastUsedAt.Valid {
		t.Error("a fresh key should have a null last_used_at")
	}

	t.Run("authenticate resolves the owner and touches last_used_at", func(t *testing.T) {
		got, err := q.AuthenticateAPIKey(ctx, liveHash)
		if err != nil {
			t.Fatalf("AuthenticateAPIKey: %v", err)
		}
		if got.UserID != alice {
			t.Errorf("user id = %d, want %d", got.UserID, alice)
		}
		var lastUsed pgtype.Timestamptz
		if err := pool.QueryRow(ctx, "SELECT last_used_at FROM api_keys WHERE id = $1", live.ID).Scan(&lastUsed); err != nil {
			t.Fatalf("read last_used_at: %v", err)
		}
		if !lastUsed.Valid {
			t.Error("last_used_at was not touched on authentication")
		}
	})

	t.Run("unknown hash returns no row", func(t *testing.T) {
		if _, err := q.AuthenticateAPIKey(ctx, "no-such-hash"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("expired key returns no row", func(t *testing.T) {
		const expiredHash = "hash-expired"
		if _, err := q.CreateAPIKey(ctx, CreateAPIKeyParams{
			UserID: alice, Name: "old", TokenHash: expiredHash, TokenPrefix: "fhk_old123", Scope: "full",
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		}); err != nil {
			t.Fatalf("CreateAPIKey(expired): %v", err)
		}
		if _, err := q.AuthenticateAPIKey(ctx, expiredHash); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("expired key err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("delete is owner-scoped", func(t *testing.T) {
		n, err := q.DeleteAPIKey(ctx, DeleteAPIKeyParams{ID: live.ID, UserID: bob})
		if err != nil {
			t.Fatalf("DeleteAPIKey(bob): %v", err)
		}
		if n != 0 {
			t.Errorf("bob deleted %d of alice's keys, want 0", n)
		}

		n, err = q.DeleteAPIKey(ctx, DeleteAPIKeyParams{ID: live.ID, UserID: alice})
		if err != nil {
			t.Fatalf("DeleteAPIKey(alice): %v", err)
		}
		if n != 1 {
			t.Errorf("alice deleted %d, want 1", n)
		}
		if _, err := q.AuthenticateAPIKey(ctx, liveHash); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("deleted key still authenticates (err = %v, want pgx.ErrNoRows)", err)
		}
	})
}
