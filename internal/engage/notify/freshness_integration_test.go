//go:build integration

// Integration tests for digest freshness: the claim reads a subscription's pending
// matches NEWEST first, a digest counts and retires what does not fit, and stale
// matches (too old, or for a closed job) leave the pending set undelivered. The
// predicates live in SQL, so only a real Postgres proves them. Run with:
// go test -tags=integration ./internal/engage/notify/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package notify

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// insertNotifyMatchAt records a pending match with an explicit matched_at.
func insertNotifyMatchAt(t *testing.T, pool *pgxpool.Pool, subscriptionID, jobID int64, matchedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO subscription_matches (subscription_id, job_id, matched_at) VALUES ($1, $2, $3)`,
		subscriptionID, jobID, matchedAt); err != nil {
		t.Fatalf("insert subscription match: %v", err)
	}
}

// matchStates reads one match's three terminal columns.
func matchStates(t *testing.T, pool *pgxpool.Pool, subscriptionID, jobID int64) (notified, failed, expired bool) {
	t.Helper()
	var n, f, e any
	if err := pool.QueryRow(context.Background(),
		`SELECT notified_at, failed_at, expired_at FROM subscription_matches WHERE subscription_id = $1 AND job_id = $2`,
		subscriptionID, jobID).Scan(&n, &f, &e); err != nil {
		t.Fatalf("load match state: %v", err)
	}
	return n != nil, f != nil, e != nil
}

func pendingCount(t *testing.T, pool *pgxpool.Pool, subscriptionID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM subscription_matches
		 WHERE subscription_id = $1 AND notified_at IS NULL AND failed_at IS NULL AND expired_at IS NULL`,
		subscriptionID).Scan(&n); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	return n
}

// The claim used to read oldest first, so a subscription with more pending than one
// digest carries was always sent its OLDEST matches.
func TestClaimTakesTheNewestMatchesFirst(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "newest-first@example.test")
	subID := insertNotifyPushSubscription(t, pool, userID, insertNotifySavedSearch(t, pool, userID, "Go", "q=go"))
	now := time.Now()
	jobs := make([]int64, 5)
	for i := range jobs {
		jobs[i] = insertNotifyJob(t, pool, sequentialExternalID("newest", i), "Engineer", sequentialSlug("newest", i))
		// jobs[0] is the oldest match, jobs[4] the newest.
		insertNotifyMatchAt(t, pool, subID, jobs[i], now.Add(-time.Duration(5-i)*time.Hour))
	}

	claimed, err := queries.ClaimSubscriptionMatches(ctx, db.ClaimSubscriptionMatchesParams{
		LeaseSeconds: 600, PerSubscription: 2, BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	got := map[int64]bool{}
	for _, c := range claimed {
		got[c.JobID] = true
		if !c.MatchedAt.Valid {
			t.Errorf("claimed row %d carries no matched_at", c.JobID)
		}
	}
	if len(got) != 2 || !got[jobs[4]] || !got[jobs[3]] {
		t.Errorf("claimed %v, want the two newest (%d, %d)", got, jobs[4], jobs[3])
	}
}

func TestExpireTakesStaleMatchesOutOfThePendingSet(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "expire@example.test")
	subID := insertNotifyPushSubscription(t, pool, userID, insertNotifySavedSearch(t, pool, userID, "Go", "q=go"))
	now := time.Now()
	job := func(name string) int64 {
		return insertNotifyJob(t, pool, "expire-"+name, "Engineer", "expire-"+name)
	}

	fresh := job("fresh")
	insertNotifyMatchAt(t, pool, subID, fresh, now.Add(-time.Hour))

	old := job("old")
	insertNotifyMatchAt(t, pool, subID, old, now.Add(-4*24*time.Hour))

	closed := job("closed")
	insertNotifyMatchAt(t, pool, subID, closed, now.Add(-time.Hour))
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed); err != nil {
		t.Fatal(err)
	}

	// Already delivered long ago: a terminal state of its own, left alone.
	delivered := job("delivered")
	insertNotifyMatchAt(t, pool, subID, delivered, now.Add(-10*24*time.Hour))
	if _, err := pool.Exec(ctx, `UPDATE subscription_matches SET notified_at = now() WHERE job_id = $1`, delivered); err != nil {
		t.Fatal(err)
	}

	// Stale, but leased by another pass right now: that pass owns it.
	leased := job("leased")
	insertNotifyMatchAt(t, pool, subID, leased, now.Add(-5*24*time.Hour))
	if _, err := pool.Exec(ctx, `UPDATE subscription_matches SET claimed_at = now() WHERE job_id = $1`, leased); err != nil {
		t.Fatal(err)
	}

	n, err := queries.ExpireStaleSubscriptionMatches(ctx, db.ExpireStaleSubscriptionMatchesParams{
		LeaseSeconds: 600, FreshnessSeconds: int32((72 * time.Hour).Seconds()), BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 2 {
		t.Errorf("expired %d, want 2 (the old one and the closed one)", n)
	}
	for _, c := range []struct {
		name                      string
		job                       int64
		notified, failed, expired bool
	}{
		{"fresh", fresh, false, false, false},
		{"old", old, false, false, true},
		{"closed", closed, false, false, true},
		{"delivered", delivered, true, false, false},
		{"leased", leased, false, false, false},
	} {
		nt, f, e := matchStates(t, pool, subID, c.job)
		if nt != c.notified || f != c.failed || e != c.expired {
			t.Errorf("%s: notified=%v failed=%v expired=%v, want %v/%v/%v",
				c.name, nt, f, e, c.notified, c.failed, c.expired)
		}
	}

	// Once the lease lapses the leased one goes too, and the backlog metric's age
	// stops counting stale matches at all.
	if _, err := pool.Exec(ctx, `UPDATE subscription_matches SET claimed_at = NULL WHERE job_id = $1`, leased); err != nil {
		t.Fatal(err)
	}
	if n, err := queries.ExpireStaleSubscriptionMatches(ctx, db.ExpireStaleSubscriptionMatchesParams{
		LeaseSeconds: 600, FreshnessSeconds: int32((72 * time.Hour).Seconds()), BatchSize: 100,
	}); err != nil || n != 1 {
		t.Fatalf("second expire = %d, %v; want the released one", n, err)
	}
	m, err := queries.NotifyBacklogMetrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.OldestAgeSeconds > (72 * time.Hour).Seconds() {
		t.Errorf("oldest pending age = %.0fs, want within the freshness window", m.OldestAgeSeconds)
	}
}

func TestExpireIsBoundedPerStatement(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "expire-batch@example.test")
	subID := insertNotifyPushSubscription(t, pool, userID, insertNotifySavedSearch(t, pool, userID, "Go", "q=go"))
	for i := 0; i < 5; i++ {
		jobID := insertNotifyJob(t, pool, sequentialExternalID("expire-batch", i), "Engineer", sequentialSlug("expire-batch", i))
		insertNotifyMatchAt(t, pool, subID, jobID, time.Now().Add(-30*24*time.Hour))
	}

	arg := db.ExpireStaleSubscriptionMatchesParams{LeaseSeconds: 600, FreshnessSeconds: 3600, BatchSize: 2}
	for _, want := range []int64{2, 2, 1, 0} {
		n, err := queries.ExpireStaleSubscriptionMatches(ctx, arg)
		if err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("expired %d, want %d", n, want)
		}
	}
}

// The end-to-end shape of the daily-digest bug: more pending than one digest
// carries. The digest must carry the newest, count every one of them, and leave
// nothing behind to be mailed again — including matches that share one
// matched_at, which is what a single RecordSubscriptionMatches batch writes.
func TestRunnerDeliversTheNewestAndRetiresTheRest(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "overflow@example.test")
	insertNotifyTelegramLink(t, pool, userID, 555)
	subID := insertNotifySubscription(t, pool, userID, insertNotifySavedSearch(t, pool, userID, "Go", "q=go"), ChannelTelegram)

	sameInstant := time.Now().Add(-2 * time.Hour)
	var jobs []int64
	for i := 0; i < 7; i++ {
		jobID := insertNotifyJob(t, pool, sequentialExternalID("overflow", i), "Engineer", sequentialSlug("overflow", i))
		jobs = append(jobs, jobID)
		insertNotifyMatchAt(t, pool, subID, jobID, sameInstant)
	}
	newest := insertNotifyJob(t, pool, "overflow-newest", "Newest", "overflow-newest")
	insertNotifyMatchAt(t, pool, subID, newest, time.Now().Add(-time.Minute))

	notifier := &fakeNotifier{}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 3
	r := New(queries, &fakeSearcher{}, notifier, cfg)

	if _, err := r.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("sent %d digests, want 1", len(notifier.sent))
	}
	d := notifier.sent[0]
	if d.Total != 8 || len(d.Jobs) != 3 {
		t.Fatalf("digest Total=%d Jobs=%d, want 8/3", d.Total, len(d.Jobs))
	}
	if d.SearchQuery != "q=go" {
		t.Errorf("SearchQuery = %q, want the saved search's query", d.SearchQuery)
	}
	carriesNewest := false
	for _, j := range d.Jobs {
		if j.Slug == "overflow-newest" {
			carriesNewest = true
		}
	}
	if !carriesNewest {
		t.Errorf("digest %+v does not carry the newest match", d.Jobs)
	}
	if got := pendingCount(t, pool, subID); got != 0 {
		t.Errorf("%d matches still pending, want 0 — everything the digest counted is delivered", got)
	}
	for _, j := range append(jobs, newest) {
		if n, f, e := matchStates(t, pool, subID, j); !n || f || e {
			t.Errorf("job %d: notified=%v failed=%v expired=%v, want notified only", j, n, f, e)
		}
	}

	// And a second pass has nothing left to send.
	notifier.sent = nil
	if _, err := r.Run(ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("second pass sent %d digests, want 0", len(notifier.sent))
	}
}

// A subscription nobody can receive keeps only fresh matches, so the person who
// reconnects is sent what is new rather than a month of mostly-closed postings.
func TestUndeliverableSubscriptionKeepsOnlyFreshMatches(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "no-device@example.test")
	subID := insertNotifyPushSubscription(t, pool, userID, insertNotifySavedSearch(t, pool, userID, "Go", "q=go"))
	oldJob := insertNotifyJob(t, pool, "nodevice-old", "Engineer", "nodevice-old")
	insertNotifyMatchAt(t, pool, subID, oldJob, time.Now().Add(-20*24*time.Hour))
	freshJob := insertNotifyJob(t, pool, "nodevice-fresh", "Engineer", "nodevice-fresh")
	insertNotifyMatchAt(t, pool, subID, freshJob, time.Now().Add(-time.Hour))

	r := New(queries, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())
	stats, err := r.Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Expired != 1 || stats.SoftSkips != 1 {
		t.Errorf("expired=%d soft_skips=%d, want 1/1", stats.Expired, stats.SoftSkips)
	}
	if _, _, e := matchStates(t, pool, subID, oldJob); !e {
		t.Error("the 20-day-old match is still pending")
	}
	if got := pendingCount(t, pool, subID); got != 1 {
		t.Errorf("%d pending, want only the fresh one", got)
	}
}
