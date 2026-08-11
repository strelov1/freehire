//go:build integration

// Integration tests for user_push_tokens' upsert-by-token semantics — a token
// identifies one device installation, not one user, so re-registering it
// under a different account must reassign the row rather than duplicate it.
// This is SQL behavior (ON CONFLICT, CASCADE) and can only be verified
// against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func TestPushTokenQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedAccount(t, pool, "alice-push@example.test")
	bob := seedAccount(t, pool, "bob-push@example.test")

	t.Run("a new token is registered", func(t *testing.T) {
		row, err := q.UpsertPushToken(ctx, UpsertPushTokenParams{
			UserID: alice, Token: "ExponentPushToken[aaa]", Platform: "ios",
		})
		if err != nil {
			t.Fatalf("UpsertPushToken: %v", err)
		}
		if row.UserID != alice {
			t.Errorf("user_id = %d, want %d", row.UserID, alice)
		}

		tokens, err := q.ListPushTokensForUser(ctx, alice)
		if err != nil {
			t.Fatalf("ListPushTokensForUser: %v", err)
		}
		if len(tokens) != 1 {
			t.Fatalf("tokens = %d, want 1", len(tokens))
		}
	})

	t.Run("re-registering the same token does not duplicate it", func(t *testing.T) {
		if _, err := q.UpsertPushToken(ctx, UpsertPushTokenParams{
			UserID: alice, Token: "ExponentPushToken[aaa]", Platform: "ios",
		}); err != nil {
			t.Fatalf("UpsertPushToken (re-register): %v", err)
		}
		tokens, err := q.ListPushTokensForUser(ctx, alice)
		if err != nil {
			t.Fatalf("ListPushTokensForUser: %v", err)
		}
		if len(tokens) != 1 {
			t.Errorf("tokens = %d, want still 1 (no duplicate)", len(tokens))
		}
	})

	t.Run("registering under a different account reassigns the token", func(t *testing.T) {
		if _, err := q.UpsertPushToken(ctx, UpsertPushTokenParams{
			UserID: bob, Token: "ExponentPushToken[aaa]", Platform: "ios",
		}); err != nil {
			t.Fatalf("UpsertPushToken (reassign): %v", err)
		}

		aliceTokens, err := q.ListPushTokensForUser(ctx, alice)
		if err != nil {
			t.Fatalf("ListPushTokensForUser(alice): %v", err)
		}
		if len(aliceTokens) != 0 {
			t.Errorf("alice's tokens = %d, want 0 after reassignment", len(aliceTokens))
		}

		bobTokens, err := q.ListPushTokensForUser(ctx, bob)
		if err != nil {
			t.Fatalf("ListPushTokensForUser(bob): %v", err)
		}
		if len(bobTokens) != 1 {
			t.Fatalf("bob's tokens = %d, want 1", len(bobTokens))
		}
	})

	t.Run("a user can only delete their own token", func(t *testing.T) {
		n, err := q.DeletePushToken(ctx, DeletePushTokenParams{
			UserID: alice, Token: "ExponentPushToken[aaa]",
		})
		if err != nil {
			t.Fatalf("DeletePushToken(wrong owner): %v", err)
		}
		if n != 0 {
			t.Errorf("rows deleted = %d, want 0 — alice does not own this token", n)
		}

		n, err = q.DeletePushToken(ctx, DeletePushTokenParams{
			UserID: bob, Token: "ExponentPushToken[aaa]",
		})
		if err != nil {
			t.Fatalf("DeletePushToken(owner): %v", err)
		}
		if n != 1 {
			t.Errorf("rows deleted = %d, want 1", n)
		}
	})

	t.Run("DeletePushTokenByValue removes regardless of owner", func(t *testing.T) {
		if _, err := q.UpsertPushToken(ctx, UpsertPushTokenParams{
			UserID: alice, Token: "ExponentPushToken[bbb]", Platform: "android",
		}); err != nil {
			t.Fatalf("UpsertPushToken: %v", err)
		}
		if err := q.DeletePushTokenByValue(ctx, "ExponentPushToken[bbb]"); err != nil {
			t.Fatalf("DeletePushTokenByValue: %v", err)
		}
		tokens, err := q.ListPushTokensForUser(ctx, alice)
		if err != nil {
			t.Fatalf("ListPushTokensForUser: %v", err)
		}
		if len(tokens) != 0 {
			t.Errorf("tokens = %d, want 0 after prune", len(tokens))
		}
	})

	t.Run("tokens die with their account", func(t *testing.T) {
		doomed := seedAccount(t, pool, "doomed-push@example.test")
		if _, err := q.UpsertPushToken(ctx, UpsertPushTokenParams{
			UserID: doomed, Token: "ExponentPushToken[ccc]", Platform: "ios",
		}); err != nil {
			t.Fatalf("UpsertPushToken: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, doomed); err != nil {
			t.Fatalf("delete user: %v", err)
		}
		var rows int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM user_push_tokens WHERE user_id = $1`, doomed).Scan(&rows); err != nil {
			t.Fatalf("count tokens: %v", err)
		}
		if rows != 0 {
			t.Errorf("orphan tokens = %d, want 0 (cascade)", rows)
		}
	})
}
