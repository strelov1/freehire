//go:build integration

// Integration tests for the subscription + match-ledger SQL semantics — owner-
// scoped create, idempotent match recording, the delivery lease/claim, and
// dead-lettering — which are SQL behavior and can only be verified against a real
// Postgres. Run with: go test -tags=integration ./internal/db/
// Reuses startPostgres/insertUser/insertJob/truncate from the package's other
// integration tests (same package db).
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func insertSavedSearch(t *testing.T, pool *pgxpool.Pool, userID int64, name, query string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, $2, $3) RETURNING id`,
		userID, name, query).Scan(&id)
	if err != nil {
		t.Fatalf("insert saved_search: %v", err)
	}
	return id
}

// truncateSubs clears everything the subscription tests touch. The existing
// truncate() helper only resets enrichment_outbox/jobs/companies.
func truncateSubs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE subscription_matches, subscriptions, saved_searches, telegram_links, jobs, users
		 RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate subs: %v", err)
	}
}

func TestSubscriptionCreate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("create is owner-scoped and returns the row", func(t *testing.T) {
		truncateSubs(t, pool)
		uid := insertUser(t, pool, "owner@example.test")
		ssID := insertSavedSearch(t, pool, uid, "Go remote", "q=go&work_mode=remote")

		sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{
			Channel: "telegram", SavedSearchID: ssID, UserID: uid,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if sub.UserID != uid || sub.SavedSearchID != ssID || !sub.Active {
			t.Errorf("unexpected subscription %+v", sub)
		}
	})

	t.Run("cannot subscribe to another user's saved search", func(t *testing.T) {
		truncateSubs(t, pool)
		owner := insertUser(t, pool, "a@example.test")
		other := insertUser(t, pool, "b@example.test")
		ssID := insertSavedSearch(t, pool, owner, "mine", "q=go")

		_, err := q.CreateSubscription(ctx, CreateSubscriptionParams{
			Channel: "telegram", SavedSearchID: ssID, UserID: other,
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("create for non-owned saved search: err=%v, want ErrNoRows", err)
		}
	})

	t.Run("duplicate (saved_search, channel) is rejected", func(t *testing.T) {
		truncateSubs(t, pool)
		uid := insertUser(t, pool, "dup@example.test")
		ssID := insertSavedSearch(t, pool, uid, "s", "q=go")
		p := CreateSubscriptionParams{Channel: "telegram", SavedSearchID: ssID, UserID: uid}
		if _, err := q.CreateSubscription(ctx, p); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := q.CreateSubscription(ctx, p); err == nil {
			t.Error("second create succeeded, want a unique-violation error")
		}
	})
}

func TestSubscriptionMatchLedger(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	// seed makes one active subscription and returns its id.
	seed := func(email string) int64 {
		uid := insertUser(t, pool, email)
		ssID := insertSavedSearch(t, pool, uid, "s", "q=go")
		sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: "telegram", SavedSearchID: ssID, UserID: uid})
		if err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
		return sub.ID
	}

	t.Run("recording a match is idempotent", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("ledger@example.test")
		job := insertJob(t, pool, "j1")

		first, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub, JobID: job})
		if err != nil || first != 1 {
			t.Fatalf("first record: rows=%d err=%v, want 1", first, err)
		}
		second, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub, JobID: job})
		if err != nil || second != 0 {
			t.Errorf("second record: rows=%d err=%v, want 0 (already recorded)", second, err)
		}
	})

	t.Run("claim leases pending rows; concurrent claims are disjoint", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("claim@example.test")
		j1 := insertJob(t, pool, "c1")
		j2 := insertJob(t, pool, "c2")
		for _, j := range []int64{j1, j2} {
			if _, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub, JobID: j}); err != nil {
				t.Fatal(err)
			}
		}
		first, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, BatchSize: 1})
		if err != nil || len(first) != 1 {
			t.Fatalf("first claim: rows=%d err=%v, want 1", len(first), err)
		}
		second, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(second) != 1 {
			t.Fatalf("second claim: rows=%d err=%v, want 1 (the other row)", len(second), err)
		}
		if first[0].JobID == second[0].JobID {
			t.Errorf("both claims returned job %d — not disjoint", first[0].JobID)
		}
		third, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(third) != 0 {
			t.Errorf("third claim: rows=%d, want 0 (all leased)", len(third))
		}
	})

	t.Run("a stale lease is reclaimable", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("stale@example.test")
		job := insertJob(t, pool, "s1")
		if _, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub, JobID: job}); err != nil {
			t.Fatal(err)
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(c), err)
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Fatalf("re-claim within lease: rows=%d, want 0", len(c))
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Errorf("re-claim with expired lease: rows=%d err=%v, want 1", len(c), err)
		}
	})

	t.Run("notified rows leave the pending queue", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("notified@example.test")
		job := insertJob(t, pool, "n1")
		if _, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub, JobID: job}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		n, err := q.MarkMatchesNotified(ctx, MarkMatchesNotifiedParams{SubscriptionID: sub, JobIds: []int64{job}})
		if err != nil || n != 1 {
			t.Fatalf("mark notified: rows=%d err=%v, want 1", n, err)
		}
		// Even with an expired lease, a notified row is never claimed again.
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Errorf("claim after notify: rows=%d, want 0", len(c))
		}
	})

	t.Run("delivery failures dead-letter at the max", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("dead@example.test")
		job := insertJob(t, pool, "d1")
		if _, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub, JobID: job}); err != nil {
			t.Fatal(err)
		}
		fail := RecordMatchDeliveryFailureParams{SubscriptionID: sub, JobIds: []int64{job}, LastError: "boom", MaxAttempts: 2}
		if err := q.RecordMatchDeliveryFailure(ctx, fail); err != nil {
			t.Fatal(err)
		}
		// One failure: still retryable (claimable with an expired lease).
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Fatalf("claim after 1 failure: rows=%d, want 1", len(c))
		}
		if err := q.RecordMatchDeliveryFailure(ctx, fail); err != nil {
			t.Fatal(err)
		}
		// Second failure reaches the max → dead-lettered, never claimed again.
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Errorf("claim after dead-letter: rows=%d, want 0", len(c))
		}
	})

	t.Run("inactive subscriptions are not claimed", func(t *testing.T) {
		truncateSubs(t, pool)
		uid := insertUser(t, pool, "inactive@example.test")
		ssID := insertSavedSearch(t, pool, uid, "s", "q=go")
		sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: "telegram", SavedSearchID: ssID, UserID: uid})
		if err != nil {
			t.Fatal(err)
		}
		job := insertJob(t, pool, "i1")
		if _, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub.ID, JobID: job}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.SetSubscriptionActive(ctx, SetSubscriptionActiveParams{Active: false, ID: sub.ID, UserID: uid}); err != nil {
			t.Fatal(err)
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Errorf("claim for inactive subscription: rows=%d, want 0", len(c))
		}
	})

	t.Run("deleting a subscription cascades its matches", func(t *testing.T) {
		truncateSubs(t, pool)
		uid := insertUser(t, pool, "cascade@example.test")
		ssID := insertSavedSearch(t, pool, uid, "s", "q=go")
		sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: "telegram", SavedSearchID: ssID, UserID: uid})
		if err != nil {
			t.Fatal(err)
		}
		job := insertJob(t, pool, "x1")
		if _, err := q.RecordSubscriptionMatch(ctx, RecordSubscriptionMatchParams{SubscriptionID: sub.ID, JobID: job}); err != nil {
			t.Fatal(err)
		}
		rows, err := q.DeleteSubscription(ctx, DeleteSubscriptionParams{ID: sub.ID, UserID: uid})
		if err != nil || rows != 1 {
			t.Fatalf("delete: rows=%d err=%v, want 1", rows, err)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM subscription_matches").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("matches after subscription delete = %d, want 0 (cascade)", n)
		}
	})
}

func TestTelegramLink(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("upsert links then relinks", func(t *testing.T) {
		truncateSubs(t, pool)
		uid := insertUser(t, pool, "tg@example.test")
		if err := q.UpsertTelegramLink(ctx, UpsertTelegramLinkParams{UserID: uid, ChatID: 111}); err != nil {
			t.Fatal(err)
		}
		link, err := q.GetTelegramLink(ctx, uid)
		if err != nil || link.ChatID != 111 {
			t.Fatalf("after link: chat=%d err=%v, want 111", link.ChatID, err)
		}
		if err := q.UpsertTelegramLink(ctx, UpsertTelegramLinkParams{UserID: uid, ChatID: 222}); err != nil {
			t.Fatal(err)
		}
		link, err = q.GetTelegramLink(ctx, uid)
		if err != nil || link.ChatID != 222 {
			t.Errorf("after relink: chat=%d err=%v, want 222", link.ChatID, err)
		}
	})

	t.Run("delete unlinks", func(t *testing.T) {
		truncateSubs(t, pool)
		uid := insertUser(t, pool, "unlink@example.test")
		if err := q.UpsertTelegramLink(ctx, UpsertTelegramLinkParams{UserID: uid, ChatID: 1}); err != nil {
			t.Fatal(err)
		}
		rows, err := q.DeleteTelegramLink(ctx, uid)
		if err != nil || rows != 1 {
			t.Fatalf("delete: rows=%d err=%v, want 1", rows, err)
		}
		if _, err := q.GetTelegramLink(ctx, uid); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("get after delete: err=%v, want ErrNoRows", err)
		}
	})
}
