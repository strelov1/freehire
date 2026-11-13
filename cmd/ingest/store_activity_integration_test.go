//go:build integration

// Release B of the job_activity split, seen from the ingest store: an ordinary re-observation of a
// posting writes ONLY job_activity, so the wide jobs tuple keeps its xmin, xmax and ctid through a
// whole Save/Touch/RefreshCovered, while the lifecycle, content and metadata changes that race
// with it still win (the state fence), and nothing is lost or counted twice.
// Run with: go test -tags=integration ./cmd/ingest/ (PostgreSQL 18, TESTDB_ADMIN_URL).
package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// activityPosting is a board posting whose provider takes no request-captured apply form and no
// Adzuna description fetch, so a re-crawl's post-commit queues cannot touch the jobs tuple: the
// tuple invariants below then say something about the write path alone.
func activityPosting(externalID, title string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "himalayas",
			ExternalID:  externalID,
			Title:       title,
			Company:     "Acme",
			Location:    "Berlin, Germany",
			Description: "<p>We are looking for a backend engineer to build services.</p>",
		},
		URL: "https://himalayas.app/acme/" + externalID,
	})
	if err != nil {
		panic(err)
	}
	return j
}

// tuple is a row's physical identity. xmax is compared against its earlier value, never against
// zero: the activity row's foreign key already stamps the inserting transaction into xmax.
type tuple struct{ xmin, xmax, ctid string }

func jobsTuple(t *testing.T, pool *pgxpool.Pool, externalID string) tuple {
	t.Helper()
	var tp tuple
	if err := pool.QueryRow(context.Background(),
		`SELECT xmin::text, xmax::text, ctid::text FROM jobs WHERE source = 'himalayas' AND external_id = $1`, externalID,
	).Scan(&tp.xmin, &tp.xmax, &tp.ctid); err != nil {
		t.Fatalf("read tuple of %s: %v", externalID, err)
	}
	return tp
}

func activityXmin(t *testing.T, pool *pgxpool.Pool, externalID string) string {
	t.Helper()
	var x string
	if err := pool.QueryRow(context.Background(),
		`SELECT a.xmin::text FROM job_activity a JOIN jobs j ON j.id = a.job_id
		 WHERE j.source = 'himalayas' AND j.external_id = $1`, externalID).Scan(&x); err != nil {
		t.Fatalf("read activity xmin of %s: %v", externalID, err)
	}
	return x
}

// seedEnriched stores a posting and marks it fully enriched, so the enrichment enqueue (the one
// statement of a save that can insert a foreign-key child row) matches nothing. The activity row
// is aged so an observation is visible as a move.
func seedEnriched(t *testing.T, pool *pgxpool.Pool, store *dbStore, externalID string) job.Job {
	t.Helper()
	posting := activityPosting(externalID, "Backend Engineer")
	if err := store.Save(context.Background(), posting); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
	for _, stmt := range []string{
		`UPDATE jobs SET enriched_at = now(), enrichment_version = 1 WHERE source = 'himalayas' AND external_id = $1`,
		`UPDATE job_activity a SET last_seen_at = now() - interval '10 days', hydrated_at = now() - interval '10 days'
		 FROM jobs j WHERE j.id = a.job_id AND j.source = 'himalayas' AND j.external_id = $1`,
	} {
		if _, err := pool.Exec(context.Background(), stmt, externalID); err != nil {
			t.Fatalf("seed stamps: %v", err)
		}
	}
	return posting
}

func activitySeenAge(t *testing.T, pool *pgxpool.Pool, externalID string) time.Duration {
	t.Helper()
	var seen time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT a.last_seen_at FROM job_activity a JOIN jobs j ON j.id = a.job_id
		 WHERE j.source = 'himalayas' AND j.external_id = $1`, externalID).Scan(&seen); err != nil {
		t.Fatalf("read activity of %s: %v", externalID, err)
	}
	return time.Since(seen)
}

// The point of the change, over the WHOLE transaction of a formless save of an enriched posting.
func TestSave_UnchangedRecrawlLeavesTheJobsTupleAlone(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), newWriteTally(), defaultSeenPolicy())
	posting := seedEnriched(t, pool, store, "act:save-1")
	before, activityBefore := jobsTuple(t, pool, "act:save-1"), activityXmin(t, pool, "act:save-1")

	if err := store.Save(ctx, posting); err != nil {
		t.Fatalf("re-crawl Save: %v", err)
	}

	if after := jobsTuple(t, pool, "act:save-1"); after != before {
		t.Errorf("jobs tuple moved over an unchanged re-crawl: before %+v, after %+v", before, after)
	}
	if activityXmin(t, pool, "act:save-1") == activityBefore {
		t.Error("job_activity tuple did not move: the observation was not written")
	}
	if age := activitySeenAge(t, pool, "act:save-1"); age > time.Minute {
		t.Errorf("last_seen_at is %s old after a re-crawl", age)
	}
}

func TestTouch_LiveRowLeavesTheJobsTupleAlone(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), newWriteTally(), defaultSeenPolicy())
	seedEnriched(t, pool, store, "act:touch-1")
	before := jobsTuple(t, pool, "act:touch-1")

	if _, err := store.Touch(ctx, "himalayas", "act:touch-1"); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	if after := jobsTuple(t, pool, "act:touch-1"); after != before {
		t.Errorf("jobs tuple moved over a live touch: before %+v, after %+v", before, after)
	}
	if age := activitySeenAge(t, pool, "act:touch-1"); age > time.Minute {
		t.Errorf("last_seen_at is %s old after a touch", age)
	}
}

func TestRefreshCovered_LeavesTheJobsTupleAlone(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), newWriteTally(), defaultSeenPolicy())
	seedEnriched(t, pool, store, "act:covered-1")
	before := jobsTuple(t, pool, "act:covered-1")

	if _, err := store.RefreshCovered(ctx, "himalayas", []string{"act:covered-1"}); err != nil {
		t.Fatalf("RefreshCovered: %v", err)
	}

	if after := jobsTuple(t, pool, "act:covered-1"); after != before {
		t.Errorf("jobs tuple moved over a covered refresh: before %+v, after %+v", before, after)
	}
	if age := activitySeenAge(t, pool, "act:covered-1"); age > time.Minute {
		t.Errorf("last_seen_at is %s old after a covered refresh", age)
	}
}

// --- races: a change that commits while the re-crawl waits must win --------------------------------

// waitForLockWait returns once some backend of this test's database waits on a lock: the re-crawl
// has taken its snapshot and is queued behind the writer the test still holds open.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool, done <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("the re-crawl finished (%v) without waiting for the open writer", err)
		case <-ctx.Done():
			t.Fatal("the re-crawl never waited for the open writer")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// raceSave runs store.Save of posting while writer's statements (an open transaction) are still
// uncommitted, and commits that transaction once Save is waiting behind it.
func raceSave(t *testing.T, pool *pgxpool.Pool, store *dbStore, posting job.Job, writer func(tx db.DBTX)) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	writer(tx)

	done := make(chan error, 1)
	go func() { done <- store.Save(ctx, posting) }()
	waitForLockWait(t, pool, done)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Save after the writer committed: %v", err)
	}
}

func TestSave_CloseThatCommitsDuringTheRecrawlWins(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool, 1, newCrawledSet(), newWriteTally(), defaultSeenPolicy())
	posting := seedEnriched(t, pool, store, "act:race-close")

	raceSave(t, pool, store, posting, func(tx db.DBTX) {
		if _, err := tx.Exec(context.Background(),
			`UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE source = 'himalayas' AND external_id = 'act:race-close'`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(context.Background(),
			`UPDATE job_activity a SET liveness_strikes = 2 FROM jobs j WHERE j.id = a.job_id AND j.external_id = 'act:race-close'`); err != nil {
			t.Fatal(err)
		}
	})

	// The crawl listed the posting after the close: the full write reopens it and clears strikes.
	var closed bool
	var strikes int32
	if err := pool.QueryRow(context.Background(),
		`SELECT j.closed_at IS NOT NULL, a.liveness_strikes FROM jobs j JOIN job_activity a ON a.job_id = j.id
		 WHERE j.external_id = 'act:race-close'`).Scan(&closed, &strikes); err != nil {
		t.Fatal(err)
	}
	if closed || strikes != 0 {
		t.Errorf("after the race: closed=%t strikes=%d, want reopened with strikes cleared", closed, strikes)
	}
	if age := activitySeenAge(t, pool, "act:race-close"); age > time.Minute {
		t.Errorf("last_seen_at is %s old: the re-listing was not recorded", age)
	}
	// One persisted posting counted once (the seed's insert and the reopening full write), and the
	// refused attempt reported beside it; the rolled-back attempt left no scope or share behind.
	if got, want := store.tally.summary(), "himalayas cheap=0/2 (0%) fenced=1"; got != want {
		t.Errorf("tally = %q, want %q", got, want)
	}
}

func TestSave_ContentEditThatCommitsDuringTheRecrawlWins(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool, 1, newCrawledSet(), newWriteTally(), defaultSeenPolicy())
	posting := seedEnriched(t, pool, store, "act:race-content")

	raceSave(t, pool, store, posting, func(tx db.DBTX) {
		if _, err := tx.Exec(context.Background(),
			`UPDATE jobs SET content_hash = 'edited-elsewhere' WHERE source = 'himalayas' AND external_id = 'act:race-content'`); err != nil {
			t.Fatal(err)
		}
	})

	// The stored row no longer matches the cheap key, so the crawl's content is written in full.
	var hash string
	if err := pool.QueryRow(context.Background(),
		`SELECT content_hash FROM jobs WHERE external_id = 'act:race-content'`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	want := posting.Fields().UpsertParams().ContentHash.String
	if hash != want {
		t.Errorf("content_hash = %q, want the crawl's %q written by the full upsert", hash, want)
	}
	if got, want := store.tally.summary(), "himalayas cheap=0/2 (0%) fenced=1"; got != want {
		t.Errorf("tally = %q, want %q", got, want)
	}
}

// RekeyCompanySlugChunk moves a retired slug's jobs onto the canonical one. A crawl that waited
// behind it must neither record the OLD slug as crawled nor overwrite the canonical slug with the
// stale one its params carry: the metadata change is fenced and the crawl retries as a fresh
// cheap write.
func TestSave_CompanyRekeyThatCommitsDuringTheRecrawlWins(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	posting := seedEnriched(t, pool, store, "act:race-rekey")
	old := posting.Fields().CompanySlug
	// A fresh set after seeding, so it holds only what the racing re-crawl records.
	crawled := newCrawledSet()
	store.crawled = crawled

	raceSave(t, pool, store, posting, func(tx db.DBTX) {
		if _, err := db.New(tx).RekeyCompanySlugChunk(context.Background(), db.RekeyCompanySlugChunkParams{
			CanonicalSlug: "acme-canonical", AliasSlug: old, ChunkSize: 100,
		}); err != nil {
			t.Fatal(err)
		}
	})

	var slug string
	if err := pool.QueryRow(context.Background(),
		`SELECT company_slug FROM jobs WHERE external_id = 'act:race-rekey'`).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "acme-canonical" {
		t.Errorf("company_slug = %q, want the canonical slug kept (a stale full upsert overwrote it)", slug)
	}
	if got := crawled.slugs("himalayas"); len(got) != 1 || got[0] != "acme-canonical" {
		t.Errorf("crawled slugs = %v, want [acme-canonical]", got)
	}
}

// --- live touch gates -------------------------------------------------------------------------------

// A posting whose stored deadline passed is not reopened by a touch, but the listing still counts
// as an observation: last_seen_at moves and the strikes a closed row carried are cleared, exactly
// what the old single statement did.
func TestTouch_ExpiredClosedPostingStaysClosedButIsObserved(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	crawled := newCrawledSet()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	seedEnriched(t, pool, store, "act:touch-expired")
	store.crawled = crawled
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET application_deadline = current_date - 3, closed_at = now() - interval '2 days', closed_reason = 'expired'
		 WHERE external_id = 'act:touch-expired'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE job_activity a SET liveness_strikes = 2 FROM jobs j WHERE j.id = a.job_id AND j.external_id = 'act:touch-expired'`); err != nil {
		t.Fatal(err)
	}

	slug, err := store.Touch(ctx, "himalayas", "act:touch-expired")
	if err != nil {
		t.Fatalf("Touch: %v", err)
	}

	var closed bool
	var reason string
	var strikes int32
	if err := pool.QueryRow(ctx, `SELECT j.closed_at IS NOT NULL, j.closed_reason, a.liveness_strikes
		FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.external_id = 'act:touch-expired'`).Scan(&closed, &reason, &strikes); err != nil {
		t.Fatal(err)
	}
	if !closed || reason != "expired" {
		t.Errorf("closed=%t reason=%q, want the expired posting to stay closed", closed, reason)
	}
	if strikes != 0 {
		t.Errorf("strikes = %d, want cleared (the old touch reset them whenever the row was closed)", strikes)
	}
	if age := activitySeenAge(t, pool, "act:touch-expired"); age > time.Minute {
		t.Errorf("last_seen_at is %s old after the touch", age)
	}
	if got := crawled.slugs("himalayas"); len(got) != 1 || got[0] != slug {
		t.Errorf("crawled = %v, want the touched company %q recorded once", got, slug)
	}
}

// A closed posting that reappears is reopened by the touch's second transaction, observed and
// requeued; its strikes clear.
func TestTouch_ClosedPostingReopensAndIsObserved(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	seedEnriched(t, pool, store, "act:touch-reopen")
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now() - interval '2 days', closed_reason = 'unseen' WHERE external_id = 'act:touch-reopen'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE job_activity a SET liveness_strikes = 2 FROM jobs j WHERE j.id = a.job_id AND j.external_id = 'act:touch-reopen'`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Touch(ctx, "himalayas", "act:touch-reopen"); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	var closed bool
	var strikes int32
	if err := pool.QueryRow(ctx, `SELECT j.closed_at IS NOT NULL, a.liveness_strikes
		FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.external_id = 'act:touch-reopen'`).Scan(&closed, &strikes); err != nil {
		t.Fatal(err)
	}
	if closed || strikes != 0 {
		t.Errorf("closed=%t strikes=%d, want reopened with strikes cleared", closed, strikes)
	}
	if age := activitySeenAge(t, pool, "act:touch-reopen"); age > time.Minute {
		t.Errorf("last_seen_at is %s old after the reopening touch", age)
	}
}

// --- covered refresh: bulk, then one waiting retry per unwritten id -------------------------------

func TestRefreshCovered_RetriesARowAWriterHeldAndRecordsItsCompanyOnce(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	for _, id := range []string{"act:cov-a", "act:cov-b", "act:cov-c"} {
		seedEnriched(t, pool, store, id)
	}
	crawled := newCrawledSet()
	store.crawled = crawled

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A writer holds one row (and, through the state trigger, its activity row) uncommitted.
	if _, err := tx.Exec(ctx, `UPDATE jobs SET content_hash = 'held' WHERE external_id = 'act:cov-b'`); err != nil {
		t.Fatal(err)
	}
	type result struct {
		slugs []string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		slugs, err := store.RefreshCovered(ctx, "himalayas", []string{"act:cov-a", "act:cov-b", "act:cov-c"})
		done <- result{slugs, err}
	}()
	waitForLockWait(t, pool, nil)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	res := <-done
	if res.err != nil {
		t.Fatalf("RefreshCovered: %v", res.err)
	}
	if len(res.slugs) != 3 {
		t.Errorf("refreshed companies = %v, want one per listed row (3)", res.slugs)
	}
	for _, id := range []string{"act:cov-a", "act:cov-b", "act:cov-c"} {
		if age := activitySeenAge(t, pool, id); age > time.Minute {
			t.Errorf("%s: last_seen_at is %s old, want refreshed", id, age)
		}
	}
	if got := crawled.slugs("himalayas"); len(got) != 1 {
		t.Errorf("crawled = %v, want the one company recorded once", got)
	}
}

// The writer never lets go within the caller's deadline: the refresh fails loudly instead of
// returning a shorter list, so the pipeline counts the board's refresh as failed and does not
// prove coverage with a listed row unrefreshed. No company joins the sweep's scope.
func TestRefreshCovered_ErrorsWhenARetryCannotFinishAndRecordsNoCompany(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	for _, id := range []string{"act:covx-a", "act:covx-b"} {
		seedEnriched(t, pool, store, id)
	}
	crawled := newCrawledSet()
	store.crawled = crawled

	bg := context.Background()
	tx, err := pool.Begin(bg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(bg) }()
	if _, err := tx.Exec(bg, `UPDATE jobs SET content_hash = 'held' WHERE external_id = 'act:covx-b'`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(bg, 800*time.Millisecond)
	defer cancel()

	slugs, err := store.RefreshCovered(ctx, "himalayas", []string{"act:covx-a", "act:covx-b"})
	if err == nil {
		t.Fatalf("RefreshCovered returned %v with no error while a listed row stayed unrefreshed", slugs)
	}
	if !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		t.Errorf("err = %v, want the deadline to be the failure", err)
	}
	if got := crawled.slugs("himalayas"); len(got) != 0 {
		t.Errorf("crawled = %v, want no company recorded after a failed refresh", got)
	}
}

// waitForLockWaits returns once at least n backends of this test's database wait on a lock.
func waitForLockWaits(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		var got int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got >= n {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("only %d of %d backends ever waited on a lock", got, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// The live touch waits behind a writer, loses the state fence, and falls back to the full touch.
// The fast statement is autocommit, so it has ended (and released the activity row) before the
// fallback takes the jobs row: a closer queued behind the same writer holds the jobs row and wants
// the activity row, and a fallback that still held the activity row would deadlock with it. The
// outcome must be fresh whichever of the two finishes first: the company the rekey committed, the
// scope recorded under it, the row open, observed, strikes clear.
func TestTouch_FencedByAConcurrentEditFallsBackWithoutHoldingTheActivityRow(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	posting := seedEnriched(t, pool, store, "act:touch-fenced")
	old := posting.Fields().CompanySlug
	crawled := newCrawledSet()
	store.crawled = crawled

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	if _, err := db.New(writer).RekeyCompanySlugChunk(ctx, db.RekeyCompanySlugChunkParams{
		CanonicalSlug: "acme-canonical", AliasSlug: old, ChunkSize: 100,
	}); err != nil {
		t.Fatal(err)
	}

	type touched struct {
		slug string
		err  error
	}
	touch := make(chan touched, 1)
	go func() {
		slug, err := store.Touch(ctx, "himalayas", "act:touch-fenced")
		touch <- touched{slug, err}
	}()
	waitForLockWaits(t, pool, 1)
	// The closer's candidate read sees the row stale (its activity was aged by the seed), locks
	// the jobs row first and queues behind the writer too.
	closed := make(chan error, 1)
	go func() {
		_, err := db.New(pool).CloseUnseenJobsBySource(ctx, db.CloseUnseenJobsBySourceParams{
			Source: "himalayas", Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
		})
		closed <- err
	}()
	waitForLockWaits(t, pool, 2)
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	res := <-touch
	if res.err != nil {
		t.Fatalf("Touch: %v (a reversed lock order shows up here as a deadlock)", res.err)
	}
	if err := <-closed; err != nil {
		t.Fatalf("closer: %v (a reversed lock order shows up here as a deadlock)", err)
	}
	if res.slug != "acme-canonical" {
		t.Errorf("Touch returned company %q, want the committed canonical slug (the stale snapshot's slug means the fence did not fall back)", res.slug)
	}
	if got := crawled.slugs("himalayas"); len(got) != 1 || got[0] != "acme-canonical" {
		t.Errorf("crawled = %v, want [acme-canonical]", got)
	}
	var closedNow bool
	var strikes int32
	var slug string
	if err := pool.QueryRow(ctx, `SELECT j.closed_at IS NOT NULL, a.liveness_strikes, j.company_slug
		FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.external_id = 'act:touch-fenced'`).Scan(&closedNow, &strikes, &slug); err != nil {
		t.Fatal(err)
	}
	if closedNow || strikes != 0 || slug != "acme-canonical" {
		t.Errorf("final row: closed=%t strikes=%d company=%q; want open, clear, canonical", closedNow, strikes, slug)
	}
	if age := activitySeenAge(t, pool, "act:touch-fenced"); age > time.Minute {
		t.Errorf("last_seen_at is %s old: the touch's observation was lost", age)
	}
}
