//go:build integration

// Integration tests for the discord_links query semantics — the one-Discord-account-to-one-
// freehire-account constraint, the reconciliation page and its rotation, and the grant
// stamp. All four are SQL behaviour and can only be verified against a real Postgres. Run
// with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedDiscordUser creates an account with a plan reaching until `proUntil`. The granted
// source column is written rather than the Stripe one because this is a hand-made fixture
// and no provider sold it — pro_until itself is generated and refuses assignment.
func seedDiscordUser(t *testing.T, pool *pgxpool.Pool, email string, proUntil *time.Time) int64 {
	t.Helper()
	var id int64
	var until pgtype.Timestamptz
	if proUntil != nil {
		until = pgtype.Timestamptz{Time: *proUntil, Valid: true}
	}
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified, pro_until_granted) VALUES ($1, true, $2) RETURNING id`,
		email, until).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func TestDiscordLinkQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	future := time.Now().Add(30 * 24 * time.Hour)
	paying := seedDiscordUser(t, pool, "paying@example.test", &future)
	free := seedDiscordUser(t, pool, "free@example.test", nil)

	t.Run("linking stores the binding", func(t *testing.T) {
		got, err := q.LinkDiscordAccount(ctx, LinkDiscordAccountParams{
			UserID: paying, DiscordUserID: "1000000000000000001",
		})
		if err != nil {
			t.Fatalf("LinkDiscordAccount: %v", err)
		}
		if got.DiscordUserID != "1000000000000000001" {
			t.Errorf("discord id = %q, want %q", got.DiscordUserID, "1000000000000000001")
		}
		if got.RoleGrantedAt.Valid {
			t.Error("a fresh link must not claim the role is already granted")
		}
	})

	t.Run("a second freehire account cannot take the same Discord account", func(t *testing.T) {
		_, err := q.LinkDiscordAccount(ctx, LinkDiscordAccountParams{
			UserID: free, DiscordUserID: "1000000000000000001",
		})
		if err == nil {
			t.Fatal("want a unique violation, got no error — one subscription would serve two people")
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("want unique_violation (23505), got %v", err)
		}
	})

	t.Run("relinking the same account moves it and forgets the grant", func(t *testing.T) {
		if err := q.SetDiscordRoleGranted(ctx, SetDiscordRoleGrantedParams{
			UserID: paying, Granted: true,
		}); err != nil {
			t.Fatalf("SetDiscordRoleGranted: %v", err)
		}
		got, err := q.LinkDiscordAccount(ctx, LinkDiscordAccountParams{
			UserID: paying, DiscordUserID: "1000000000000000002",
		})
		if err != nil {
			t.Fatalf("relink: %v", err)
		}
		if got.DiscordUserID != "1000000000000000002" {
			t.Errorf("discord id = %q, want the new one", got.DiscordUserID)
		}
		if got.RoleGrantedAt.Valid {
			t.Error("a relink points at a different Discord account, which holds no role yet")
		}
	})

	// The bug this guards: reconciliation stamps EVERY row it examines, including the settled
	// ones that need no Discord call. If the stamp also rewrote role_granted_at, a paying
	// account's grant time would creep forward every hour — the column would stop meaning
	// "when the role was granted" and become a second, redundant copy of synced_at.
	t.Run("re-recording a held role does not move when it was granted", func(t *testing.T) {
		if err := q.SetDiscordRoleGranted(ctx, SetDiscordRoleGrantedParams{
			UserID: paying, Granted: true,
		}); err != nil {
			t.Fatalf("first grant: %v", err)
		}
		first, err := q.GetDiscordLink(ctx, paying)
		if err != nil {
			t.Fatalf("GetDiscordLink: %v", err)
		}
		if !first.RoleGrantedAt.Valid {
			t.Fatal("granting must record an instant")
		}

		if err := q.SetDiscordRoleGranted(ctx, SetDiscordRoleGrantedParams{
			UserID: paying, Granted: true,
		}); err != nil {
			t.Fatalf("second grant: %v", err)
		}
		second, err := q.GetDiscordLink(ctx, paying)
		if err != nil {
			t.Fatalf("GetDiscordLink: %v", err)
		}
		if !second.RoleGrantedAt.Time.Equal(first.RoleGrantedAt.Time) {
			t.Errorf("role_granted_at moved %v → %v on a re-record", first.RoleGrantedAt.Time, second.RoleGrantedAt.Time)
		}
		// The examination stamp DOES move — that is what rotates the queue.
		if !second.SyncedAt.Time.After(first.SyncedAt.Time) {
			t.Error("synced_at did not move, so the row would pin the front of the queue")
		}

		// And a revocation clears the grant outright.
		if err := q.SetDiscordRoleGranted(ctx, SetDiscordRoleGrantedParams{
			UserID: paying, Granted: false,
		}); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		after, err := q.GetDiscordLink(ctx, paying)
		if err != nil {
			t.Fatalf("GetDiscordLink: %v", err)
		}
		if after.RoleGrantedAt.Valid {
			t.Error("a revoked role must leave no grant instant behind")
		}
	})

	t.Run("the sync page carries the plan and rotates", func(t *testing.T) {
		if _, err := q.LinkDiscordAccount(ctx, LinkDiscordAccountParams{
			UserID: free, DiscordUserID: "1000000000000000003",
		}); err != nil {
			t.Fatalf("link free user: %v", err)
		}

		first, err := q.ListDiscordLinksToSync(ctx, 1)
		if err != nil {
			t.Fatalf("ListDiscordLinksToSync: %v", err)
		}
		if len(first) != 1 {
			t.Fatalf("page size = %d, want 1 — the bound is what keeps a run inside its timer", len(first))
		}

		// Stamping the row it returned must move it to the back of the queue, so a bounded
		// run that cannot reach everybody still reaches everybody eventually.
		if err := q.SetDiscordRoleGranted(ctx, SetDiscordRoleGrantedParams{
			UserID: first[0].UserID, Granted: false,
		}); err != nil {
			t.Fatalf("stamp: %v", err)
		}
		second, err := q.ListDiscordLinksToSync(ctx, 1)
		if err != nil {
			t.Fatalf("ListDiscordLinksToSync (second): %v", err)
		}
		if len(second) != 1 {
			t.Fatalf("page size = %d, want 1", len(second))
		}
		if second[0].UserID == first[0].UserID {
			t.Error("the same account came back twice — a bounded run would never reach the rest")
		}

		for _, row := range append(first, second...) {
			switch row.UserID {
			case paying:
				if !row.ProUntil.Valid || !row.ProUntil.Time.After(time.Now()) {
					t.Error("the paying account's plan did not come back with its link")
				}
			case free:
				if row.ProUntil.Valid && row.ProUntil.Time.After(time.Now()) {
					t.Error("the free account came back looking paid")
				}
			}
		}
	})

	t.Run("unlinking removes the binding and reports it", func(t *testing.T) {
		n, err := q.UnlinkDiscordAccount(ctx, paying)
		if err != nil {
			t.Fatalf("UnlinkDiscordAccount: %v", err)
		}
		if n != 1 {
			t.Fatalf("rows removed = %d, want 1", n)
		}
		if _, err := q.GetDiscordLink(ctx, paying); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("want pgx.ErrNoRows after unlink, got %v", err)
		}
		again, err := q.UnlinkDiscordAccount(ctx, paying)
		if err != nil {
			t.Fatalf("second UnlinkDiscordAccount: %v", err)
		}
		if again != 0 {
			t.Errorf("rows removed = %d, want 0 — unlinking twice must be harmless", again)
		}
	})

	t.Run("deleting the account takes the link with it", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", free); err != nil {
			t.Fatalf("delete user: %v", err)
		}
		var n int64
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM discord_links WHERE user_id = $1", free).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("links left behind = %d, want 0", n)
		}
	})
}
