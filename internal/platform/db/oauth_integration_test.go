//go:build integration

// Integration tests for the MCP OAuth query semantics — client registration, the
// authorization-code single-use exchange, and the grant's tie to the account's
// session generation (users.token_version) — which are SQL behavior and can only
// be verified against a real Postgres. Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestOAuthClientAndGrantQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedAPIKeyUser(t, pool, "oauth-alice@example.test")

	client, err := q.RegisterOAuthClient(ctx, RegisterOAuthClientParams{
		ClientID:     "client-test-1",
		ClientName:   "Test MCP Client",
		RedirectUris: []string{"http://127.0.0.1:9999/callback"},
	})
	if err != nil {
		t.Fatalf("RegisterOAuthClient: %v", err)
	}
	if client.ClientName != "Test MCP Client" {
		t.Errorf("client_name = %q, want %q", client.ClientName, "Test MCP Client")
	}

	t.Run("GetOAuthClient resolves a registered client", func(t *testing.T) {
		got, err := q.GetOAuthClient(ctx, "client-test-1")
		if err != nil {
			t.Fatalf("GetOAuthClient: %v", err)
		}
		if len(got.RedirectUris) != 1 || got.RedirectUris[0] != "http://127.0.0.1:9999/callback" {
			t.Errorf("redirect_uris = %v", got.RedirectUris)
		}
	})

	t.Run("GetOAuthClient on an unknown id returns no row", func(t *testing.T) {
		if _, err := q.GetOAuthClient(ctx, "no-such-client"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("authorization code is consumed exactly once", func(t *testing.T) {
		const codeHash = "code-hash-1"
		err := q.CreateOAuthAuthorizationCode(ctx, CreateOAuthAuthorizationCodeParams{
			CodeHash:      codeHash,
			ClientID:      "client-test-1",
			UserID:        alice,
			RedirectUri:   "http://127.0.0.1:9999/callback",
			CodeChallenge: "challenge-1",
			ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(10 * time.Minute), Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateOAuthAuthorizationCode: %v", err)
		}

		row, err := q.ConsumeOAuthAuthorizationCode(ctx, codeHash)
		if err != nil {
			t.Fatalf("ConsumeOAuthAuthorizationCode (first): %v", err)
		}
		if row.UserID != alice || row.ClientID != "client-test-1" || row.CodeChallenge != "challenge-1" {
			t.Errorf("consumed row = %+v", row)
		}

		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, codeHash); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("replayed code err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("an expired authorization code cannot be consumed", func(t *testing.T) {
		const codeHash = "code-hash-expired"
		err := q.CreateOAuthAuthorizationCode(ctx, CreateOAuthAuthorizationCodeParams{
			CodeHash:      codeHash,
			ClientID:      "client-test-1",
			UserID:        alice,
			RedirectUri:   "http://127.0.0.1:9999/callback",
			CodeChallenge: "challenge-2",
			ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateOAuthAuthorizationCode: %v", err)
		}
		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, codeHash); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("expired code err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("a grant authenticates while its token version matches, and stops once bumped", func(t *testing.T) {
		version, err := q.GetUserTokenVersion(ctx, alice)
		if err != nil {
			t.Fatalf("GetUserTokenVersion: %v", err)
		}

		grant, err := q.CreateOAuthGrant(ctx, CreateOAuthGrantParams{
			UserID:             alice,
			ClientID:           "client-test-1",
			AccessTokenHash:    "token-hash-1",
			IssuedTokenVersion: version,
			ExpiresAt:          pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateOAuthGrant: %v", err)
		}
		if grant.ID == 0 {
			t.Fatal("grant.ID is zero")
		}

		gotUserID, err := q.AuthenticateOAuthGrant(ctx, "token-hash-1")
		if err != nil {
			t.Fatalf("AuthenticateOAuthGrant (before bump): %v", err)
		}
		if gotUserID != alice {
			t.Errorf("user id = %d, want %d", gotUserID, alice)
		}

		if _, err := q.BumpUserTokenVersion(ctx, alice); err != nil {
			t.Fatalf("BumpUserTokenVersion: %v", err)
		}

		if _, err := q.AuthenticateOAuthGrant(ctx, "token-hash-1"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("after bump, err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("ListOAuthGrantsByUser and DeleteOAuthGrant are owner-scoped", func(t *testing.T) {
		bob := seedAPIKeyUser(t, pool, "oauth-bob@example.test")
		version, err := q.GetUserTokenVersion(ctx, bob)
		if err != nil {
			t.Fatalf("GetUserTokenVersion(bob): %v", err)
		}
		grant, err := q.CreateOAuthGrant(ctx, CreateOAuthGrantParams{
			UserID: bob, ClientID: "client-test-1", AccessTokenHash: "token-hash-bob",
			IssuedTokenVersion: version,
			ExpiresAt:          pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatalf("CreateOAuthGrant(bob): %v", err)
		}

		rows, err := q.ListOAuthGrantsByUser(ctx, bob)
		if err != nil {
			t.Fatalf("ListOAuthGrantsByUser: %v", err)
		}
		if len(rows) != 1 || rows[0].ClientName != "Test MCP Client" {
			t.Errorf("rows = %+v", rows)
		}

		n, err := q.DeleteOAuthGrant(ctx, DeleteOAuthGrantParams{ID: grant.ID, UserID: alice})
		if err != nil {
			t.Fatalf("DeleteOAuthGrant(alice): %v", err)
		}
		if n != 0 {
			t.Errorf("alice deleted %d of bob's grants, want 0", n)
		}

		n, err = q.DeleteOAuthGrant(ctx, DeleteOAuthGrantParams{ID: grant.ID, UserID: bob})
		if err != nil {
			t.Fatalf("DeleteOAuthGrant(bob): %v", err)
		}
		if n != 1 {
			t.Errorf("bob deleted %d, want 1", n)
		}
	})
}
