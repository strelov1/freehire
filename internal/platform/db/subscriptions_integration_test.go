//go:build integration

// Integration tests for the subscription + match-ledger SQL semantics — owner-
// scoped create, idempotent match recording, the delivery lease/claim, and
// dead-lettering — which are SQL behavior and can only be verified against a real
// Postgres. Run with: go test -tags=integration ./internal/platform/db/
// Reuses startPostgres/insertUser/insertJob/truncate from the package's other
// integration tests (same package db).
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

// recordMatch records a single (subscription, job) match via the batched insert
// RecordSubscriptionMatches replaced RecordSubscriptionMatch with, returning the
// affected row count (1 = newly recorded, 0 = already known) for tests exercising the
// single-pair case.
func recordMatch(ctx context.Context, q *Queries, sub, job int64) (int64, error) {
	return q.RecordSubscriptionMatches(ctx, RecordSubscriptionMatchesParams{
		SubscriptionIds: []int64{sub},
		JobIds:          []int64{job},
		QueryRevisions:  []int32{0},
	})
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

func TestGetSubscriptionForDelivery_ResolvesAccountEmail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	truncateSubs(t, pool)
	uid := insertVerifiedUser(t, pool, "digest@example.test")
	ssID := insertSavedSearch(t, pool, uid, "Go remote", "q=go")
	sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{
		Channel: "email", SavedSearchID: ssID, UserID: uid,
	})
	if err != nil {
		t.Fatalf("create email subscription: %v", err)
	}

	info, err := q.GetSubscriptionForDelivery(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetSubscriptionForDelivery: %v", err)
	}
	// The email channel's live recipient is the user's account email, joined from
	// users; with no telegram link the chat id is absent.
	if info.Channel != "email" {
		t.Errorf("channel = %q, want email", info.Channel)
	}
	if info.AccountEmail != "digest@example.test" {
		t.Errorf("account_email = %q, want digest@example.test", info.AccountEmail)
	}
	if info.TelegramChatID.Valid {
		t.Errorf("telegram_chat_id = %v, want absent (no link)", info.TelegramChatID)
	}
	if info.SavedSearchName != "Go remote" {
		t.Errorf("saved_search_name = %q, want Go remote", info.SavedSearchName)
	}
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

		first, err := recordMatch(ctx, q, sub, job)
		if err != nil || first != 1 {
			t.Fatalf("first record: rows=%d err=%v, want 1", first, err)
		}
		second, err := recordMatch(ctx, q, sub, job)
		if err != nil || second != 0 {
			t.Errorf("second record: rows=%d err=%v, want 0 (already recorded)", second, err)
		}
	})

	// The batched insert (RecordSubscriptionMatches) is the actual fix: one round trip
	// for a whole query's (subscription, job) pairs instead of one INSERT per pair.
	// This exercises it with more than one pair, a mix of new and already-known ones.
	t.Run("batched recording inserts new pairs and skips known ones", func(t *testing.T) {
		truncateSubs(t, pool)
		subA := seed("batch-a@example.test")
		subB := seed("batch-b@example.test")
		j1 := insertJob(t, pool, "b1")
		j2 := insertJob(t, pool, "b2")

		if _, err := recordMatch(ctx, q, subA, j1); err != nil {
			t.Fatalf("seed known pair: %v", err)
		}

		n, err := q.RecordSubscriptionMatches(ctx, RecordSubscriptionMatchesParams{
			SubscriptionIds: []int64{subA, subA, subB},
			JobIds:          []int64{j1, j2, j1},
			QueryRevisions:  []int32{0, 0, 0},
		})
		if err != nil {
			t.Fatalf("batched record: %v", err)
		}
		if n != 2 {
			t.Errorf("newly recorded = %d, want 2 (subA/j1 was already known)", n)
		}
		var total int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM subscription_matches").Scan(&total); err != nil {
			t.Fatal(err)
		}
		if total != 3 {
			t.Errorf("total matches = %d, want 3 (1 seeded + 2 newly batched)", total)
		}
	})

	t.Run("deleted subscriptions do not discard other subscribers' matches", func(t *testing.T) {
		truncateSubs(t, pool)
		removed := seed("removed@example.test")
		remaining := seed("remaining@example.test")
		j1 := insertJob(t, pool, "removed-batch-1")
		j2 := insertJob(t, pool, "removed-batch-2")

		// The worker has already listed both subscriptions when someone unsubscribes.
		if _, err := pool.Exec(ctx, "DELETE FROM subscriptions WHERE id=$1", removed); err != nil {
			t.Fatal(err)
		}
		args := RecordSubscriptionMatchesParams{
			SubscriptionIds: []int64{removed, remaining, remaining},
			JobIds:          []int64{j1, j1, j2},
			QueryRevisions:  []int32{0, 0, 0},
		}
		n, err := q.RecordSubscriptionMatches(ctx, args)
		if err != nil || n != 2 {
			t.Fatalf("record with deleted subscriber: rows=%d err=%v, want 2", n, err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM subscription_matches WHERE subscription_id=$1", remaining).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("remaining subscriber has %d matches, want 2", count)
		}
		if n, err := q.RecordSubscriptionMatches(ctx, args); err != nil || n != 0 {
			t.Fatalf("repeat record: rows=%d err=%v, want 0", n, err)
		}
	})

	t.Run("concurrent deletion does not discard other subscribers' matches", func(t *testing.T) {
		truncateSubs(t, pool)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		removed := seed("concurrent-removed@example.test")
		remaining := seed("concurrent-remaining@example.test")
		job := insertJob(t, pool, "concurrent-match")
		deleting, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = deleting.Rollback(context.Background()) }()
		if _, err := deleting.Exec(ctx, "DELETE FROM subscriptions WHERE id=$1", removed); err != nil {
			t.Fatal(err)
		}
		recording, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer recording.Release()
		type result struct {
			rows int64
			err  error
		}
		recorded := make(chan result, 1)
		go func() {
			n, err := New(recording).RecordSubscriptionMatches(ctx, RecordSubscriptionMatchesParams{
				SubscriptionIds: []int64{removed, remaining},
				JobIds:          []int64{job, job},
				QueryRevisions:  []int32{0, 0},
			})
			recorded <- result{n, err}
		}()
		// Prove the INSERT took its snapshot before the DELETE committed. A plain
		// existence check without a parent-row lock still races in this ordering.
		for {
			var blocked bool
			if err := pool.QueryRow(ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, recording.Conn().PgConn().PID()).Scan(&blocked); err != nil {
				t.Fatal(err)
			}
			if blocked {
				break
			}
			select {
			case r := <-recorded:
				t.Fatalf("recording finished before deletion committed: %+v", r)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
		if err := deleting.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		r := <-recorded
		if r.err != nil || r.rows != 1 {
			t.Fatalf("record after concurrent deletion: rows=%d err=%v, want 1", r.rows, r.err)
		}
		if n, err := recordMatch(ctx, q, remaining, job); err != nil || n != 0 {
			t.Fatalf("remaining subscriber's match was not recorded: rows=%d err=%v", n, err)
		}
	})

	t.Run("claim leases pending rows; concurrent claims are disjoint", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("claim@example.test")
		j1 := insertJob(t, pool, "c1")
		j2 := insertJob(t, pool, "c2")
		for _, j := range []int64{j1, j2} {
			if _, err := recordMatch(ctx, q, sub, j); err != nil {
				t.Fatal(err)
			}
		}
		first, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 1})
		if err != nil || len(first) != 1 {
			t.Fatalf("first claim: rows=%d err=%v, want 1", len(first), err)
		}
		second, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 10})
		if err != nil || len(second) != 1 {
			t.Fatalf("second claim: rows=%d err=%v, want 1 (the other row)", len(second), err)
		}
		if first[0].JobID == second[0].JobID {
			t.Errorf("both claims returned job %d — not disjoint", first[0].JobID)
		}
		third, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 10})
		if err != nil || len(third) != 0 {
			t.Errorf("third claim: rows=%d, want 0 (all leased)", len(third))
		}
	})

	t.Run("a stale lease is reclaimable", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("stale@example.test")
		job := insertJob(t, pool, "s1")
		if _, err := recordMatch(ctx, q, sub, job); err != nil {
			t.Fatal(err)
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(c), err)
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Fatalf("re-claim within lease: rows=%d, want 0", len(c))
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Errorf("re-claim with expired lease: rows=%d err=%v, want 1", len(c), err)
		}
	})

	t.Run("notified rows leave the pending queue", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("notified@example.test")
		job := insertJob(t, pool, "n1")
		if _, err := recordMatch(ctx, q, sub, job); err != nil {
			t.Fatal(err)
		}
		if _, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 10}); err != nil {
			t.Fatal(err)
		}
		n, err := q.MarkMatchesNotified(ctx, MarkMatchesNotifiedParams{SubscriptionID: sub, JobIds: []int64{job}})
		if err != nil || n != 1 {
			t.Fatalf("mark notified: rows=%d err=%v, want 1", n, err)
		}
		// Even with an expired lease, a notified row is never claimed again.
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Errorf("claim after notify: rows=%d, want 0", len(c))
		}
	})

	t.Run("delivery failures dead-letter at the max", func(t *testing.T) {
		truncateSubs(t, pool)
		sub := seed("dead@example.test")
		job := insertJob(t, pool, "d1")
		if _, err := recordMatch(ctx, q, sub, job); err != nil {
			t.Fatal(err)
		}
		fail := RecordMatchDeliveryFailureParams{SubscriptionID: sub, JobIds: []int64{job}, LastError: "boom", MaxAttempts: 2}
		if err := q.RecordMatchDeliveryFailure(ctx, fail); err != nil {
			t.Fatal(err)
		}
		// One failure: still retryable (claimable with an expired lease).
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Fatalf("claim after 1 failure: rows=%d, want 1", len(c))
		}
		if err := q.RecordMatchDeliveryFailure(ctx, fail); err != nil {
			t.Fatal(err)
		}
		// Second failure reaches the max → dead-lettered, never claimed again.
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 0 {
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
		if _, err := recordMatch(ctx, q, sub.ID, job); err != nil {
			t.Fatal(err)
		}
		if _, err := q.SetSubscriptionActive(ctx, SetSubscriptionActiveParams{Active: false, ID: sub.ID, UserID: uid}); err != nil {
			t.Fatal(err)
		}
		if c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 0, PerSubscription: 100, BatchSize: 10}); err != nil || len(c) != 0 {
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
		if _, err := recordMatch(ctx, q, sub.ID, job); err != nil {
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

func TestUpdateSavedSearchDropsPendingMatchesOnlyWhenTheQueryChanges(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	uid := insertUser(t, pool, "edit@example.test")
	ssID := insertSavedSearch(t, pool, uid, "s", "q=go")
	sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: "telegram", SavedSearchID: ssID, UserID: uid})
	if err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	pending, sent, leased := insertJob(t, pool, "pending"), insertJob(t, pool, "sent"), insertJob(t, pool, "leased")
	for _, job := range []int64{pending, sent, leased} {
		if _, err := recordMatch(ctx, q, sub.ID, job); err != nil {
			t.Fatalf("record match: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE subscription_matches SET notified_at = now() WHERE job_id = $1`, sent); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE subscription_matches SET claimed_at = now() WHERE job_id = $1`, leased); err != nil {
		t.Fatal(err)
	}
	remaining := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM subscription_matches WHERE subscription_id = $1`, sub.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	text := func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} }

	// A rename and a re-save of the same query answer the same question.
	if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Name: text("renamed")}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Query: text("q=go")}); err != nil {
		t.Fatalf("same query: %v", err)
	}
	if got := remaining(); got != 3 {
		t.Fatalf("matches after a rename and an unchanged query = %d, want 3", got)
	}

	// Another account's id must not reach this subscription's ledger.
	stranger := insertUser(t, pool, "stranger@example.test")
	if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: stranger, Query: text("q=rust")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stranger's update: err = %v, want no rows", err)
	}
	if got := remaining(); got != 3 {
		t.Fatalf("matches after a stranger's update = %d, want 3", got)
	}

	if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Query: text("q=go&min_skill_match=70")}); err != nil {
		t.Fatalf("change query: %v", err)
	}
	if got := remaining(); got != 2 {
		t.Fatalf("matches after a changed query = %d, want 2 (the sent one and the leased one)", got)
	}
	var left int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM subscription_matches WHERE job_id = $1`, pending).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("the pending match survived the query change")
	}
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

func TestSavedSearchQueryRevisionGuardsStaleMatches(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	text := func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} }
	claimAll := func(lease int32) int {
		t.Helper()
		c, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: lease, PerSubscription: 100, BatchSize: 100})
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		return len(c)
	}
	expire := func() int64 {
		t.Helper()
		n, err := q.ExpireStaleSubscriptionMatches(ctx, ExpireStaleSubscriptionMatchesParams{LeaseSeconds: 0, FreshnessSeconds: 86400, BatchSize: 100})
		if err != nil {
			t.Fatalf("expire: %v", err)
		}
		return n
	}
	record := func(sub, job int64, rev int32) int64 {
		t.Helper()
		n, err := q.RecordSubscriptionMatches(ctx, RecordSubscriptionMatchesParams{
			SubscriptionIds: []int64{sub}, JobIds: []int64{job}, QueryRevisions: []int32{rev},
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		return n
	}
	seed := func(email string) (ssID, uid int64, sub Subscription) {
		truncateSubs(t, pool)
		uid = insertUser(t, pool, email)
		ssID = insertSavedSearch(t, pool, uid, "s", "q=go")
		sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: "telegram", SavedSearchID: ssID, UserID: uid})
		if err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
		return ssID, uid, sub
	}
	revisionOf := func(ssID int64) int32 {
		t.Helper()
		var r int32
		if err := pool.QueryRow(ctx, `SELECT query_revision FROM saved_searches WHERE id = $1`, ssID).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("a match recorded after the edit under the old revision is refused", func(t *testing.T) {
		ssID, uid, sub := seed("rec@example.test")
		job := insertJob(t, pool, "j")
		rows, err := q.ListActiveSubscriptions(ctx)
		if err != nil || len(rows) != 1 {
			t.Fatalf("list active: rows=%d err=%v", len(rows), err)
		}
		oldRev := rows[0].QueryRevision
		if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Query: text("q=rust")}); err != nil {
			t.Fatal(err)
		}
		if n := record(sub.ID, job, oldRev); n != 0 {
			t.Errorf("recorded %d rows under a stale revision, want 0", n)
		}
		if got := claimAll(3600); got != 0 {
			t.Errorf("claimed %d, want 0", got)
		}
	})

	for _, tc := range []string{"released", "lease lapsed"} {
		t.Run("a leased match is not re-claimed after the edit: "+tc, func(t *testing.T) {
			ssID, uid, sub := seed("lease-" + tc[:3] + "@example.test")
			job := insertJob(t, pool, "j")
			record(sub.ID, job, revisionOf(ssID))
			if got := claimAll(3600); got != 1 {
				t.Fatalf("first claim = %d, want 1", got)
			}
			if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Query: text("q=rust")}); err != nil {
				t.Fatal(err)
			}
			lease := int32(3600)
			if tc == "released" {
				if err := q.ReleaseMatchClaim(ctx, ReleaseMatchClaimParams{SubscriptionID: sub.ID, JobIds: []int64{job}}); err != nil {
					t.Fatal(err)
				}
			} else {
				lease = 0
			}
			if got := claimAll(lease); got != 0 {
				t.Errorf("claimed %d superseded matches, want 0", got)
			}
			if n := expire(); n != 1 {
				t.Errorf("expiry retired %d rows, want 1", n)
			}
		})
	}

	t.Run("a rename keeps pending matches claimable", func(t *testing.T) {
		ssID, uid, sub := seed("rename@example.test")
		job := insertJob(t, pool, "j")
		record(sub.ID, job, revisionOf(ssID))
		if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Name: text("renamed")}); err != nil {
			t.Fatal(err)
		}
		if got := revisionOf(ssID); got != 0 {
			t.Errorf("revision after a rename = %d, want 0", got)
		}
		if got := claimAll(3600); got != 1 {
			t.Errorf("claimed %d after a rename, want 1", got)
		}
	})

	t.Run("a job matching old and new query is recorded again under the new revision", func(t *testing.T) {
		ssID, uid, sub := seed("both@example.test")
		job := insertJob(t, pool, "j")
		record(sub.ID, job, 0)
		// A leased row is kept by the edit's delete, so the conflict path must reset it.
		if got := claimAll(3600); got != 1 {
			t.Fatalf("claim = %d, want 1", got)
		}
		if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Query: text("q=go&remote=1")}); err != nil {
			t.Fatal(err)
		}
		if n := record(sub.ID, job, 1); n != 1 {
			t.Fatalf("re-record under the new revision = %d rows, want 1", n)
		}
		if got := claimAll(3600); got != 1 {
			t.Errorf("claimed %d, want 1", got)
		}
	})

	t.Run("count and mark skip superseded rows", func(t *testing.T) {
		ssID, uid, sub := seed("cm@example.test")
		old, cur := insertJob(t, pool, "old"), insertJob(t, pool, "cur")
		record(sub.ID, old, 0)
		if _, err := pool.Exec(ctx, `UPDATE subscription_matches SET claimed_at = now()`); err != nil {
			t.Fatal(err)
		}
		if _, err := q.UpdateSavedSearch(ctx, UpdateSavedSearchParams{ID: ssID, UserID: uid, Query: text("q=rust")}); err != nil {
			t.Fatal(err)
		}
		record(sub.ID, cur, 1)
		before := pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}
		n, err := q.CountCoveredMatches(ctx, CountCoveredMatchesParams{SubscriptionID: sub.ID, BeforeMatchedAt: before, BeforeJobID: 0})
		if err != nil || n != 1 {
			t.Fatalf("count = %d err=%v, want 1", n, err)
		}
		marked, err := q.MarkCoveredMatchesNotified(ctx, MarkCoveredMatchesNotifiedParams{SubscriptionID: sub.ID, BeforeMatchedAt: before, BeforeJobID: 0})
		if err != nil || marked != 1 {
			t.Fatalf("mark = %d err=%v, want 1", marked, err)
		}
		var stamped int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM subscription_matches WHERE job_id = $1 AND notified_at IS NOT NULL`, old).Scan(&stamped); err != nil || stamped != 0 {
			t.Errorf("superseded row stamped notified: n=%d err=%v", stamped, err)
		}
	})
}
