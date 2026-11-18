//go:build integration

// Release B covered refresh and the lock shapes around it: the bulk statement never waits (SKIP
// LOCKED) and reports a per-row outcome, the one-row retry waits for the writer and is fenced,
// and the full upsert is not blocked by, and never deadlocks with, a cheap transaction that holds
// the activity row and then inserts a child of the posting.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	stdmaps "maps"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedCovered stores five aged postings acme:1..5 and returns their ids by external id.
func seedCovered(t *testing.T, pool *pgxpool.Pool) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	q := New(pool)
	truncate(t, pool)
	ids := map[string]int64{}
	for _, ext := range []string{"acme:1", "acme:2", "acme:3", "acme:4", "acme:5"} {
		job, err := ingestUpsert(ctx, q, ingestParams(ext, "Covered "+ext))
		if err != nil {
			t.Fatal(err)
		}
		ids[ext] = job.ID
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '10 days'`); err != nil {
		t.Fatal(err)
	}
	return ids
}

func coveredParams(exts ...string) RefreshCoveredJobsParams {
	return RefreshCoveredJobsParams{Source: "greenhouse", ExternalIds: exts}
}

func outcomes(rows []RefreshCoveredJobsRow) map[string]bool {
	m := map[string]bool{}
	for _, r := range rows {
		m[r.ExternalID] = r.Refreshed
	}
	return m
}

func TestCoveredRefreshReportsAPerRowOutcome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := startPostgres(t)
	ids := seedCovered(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE id = $1`, ids["acme:3"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private = true WHERE id = $1`, ids["acme:4"]); err != nil {
		t.Fatal(err)
	}

	// acme:2 is held by another transaction: the bulk statement must not wait for it.
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(context.Background()) }()
	if _, err := holder.Exec(ctx, `SELECT 1 FROM job_activity WHERE job_id = $1 FOR NO KEY UPDATE`, ids["acme:2"]); err != nil {
		t.Fatal(err)
	}

	rows, err := New(pool).RefreshCoveredJobs(ctx, coveredParams("acme:1", "acme:2", "acme:3", "acme:4", "acme:5", "acme:404"))
	if err != nil {
		t.Fatalf("RefreshCoveredJobs: %v", err)
	}
	got := outcomes(rows)
	want := map[string]bool{"acme:1": true, "acme:2": false, "acme:5": true}
	if !stdmaps.Equal(got, want) {
		t.Fatalf("outcomes = %v, want %v: closed, private and absent postings come back as no row, a held row as refreshed=false", got, want)
	}
	for _, r := range rows {
		if r.CompanySlug != "acme" {
			t.Errorf("row %+v has no company: the caller records it for the sweep scope", r)
		}
	}
	if age := obsSeenAge(t, pool, ids["acme:2"]); age < 9*24*time.Hour {
		t.Errorf("the held row was written (%s old) by a statement that must skip it", age)
	}
	if age := obsSeenAge(t, pool, ids["acme:3"]); age < 9*24*time.Hour {
		t.Errorf("the closed row was refreshed (%s old): a covered refresh never touches a closed posting", age)
	}

	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	one, err := New(pool).RefreshCoveredJob(ctx, RefreshCoveredJobParams{Source: "greenhouse", ExternalID: "acme:2"})
	if err != nil || !one.Refreshed || one.ExternalID != "acme:2" || one.CompanySlug != "acme" {
		t.Fatalf("retry of the skipped row = %+v, %v; want it refreshed", one, err)
	}
	for _, ext := range []string{"acme:3", "acme:4", "acme:404"} {
		if _, err := New(pool).RefreshCoveredJob(ctx, RefreshCoveredJobParams{Source: "greenhouse", ExternalID: ext}); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("RefreshCoveredJob(%s): err = %v, want pgx.ErrNoRows (closed, private or absent)", ext, err)
		}
	}
}

// The retry waits for the writer that held the row, holding nothing else; a writer that changed
// the posting makes it report refreshed=false (the statement has ended, its lock released) and a
// fresh statement then succeeds.
func TestCoveredRetryWaitsAndIsFenced(t *testing.T) {
	for _, tc := range []struct {
		name        string
		hold        func(t *testing.T, ctx context.Context, tx pgx.Tx, id int64)
		wantFirstOK bool
	}{
		{"a lock only", func(t *testing.T, ctx context.Context, tx pgx.Tx, id int64) {
			if _, err := tx.Exec(ctx, `SELECT 1 FROM job_activity WHERE job_id = $1 FOR NO KEY UPDATE`, id); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"a content change", func(t *testing.T, ctx context.Context, tx pgx.Tx, id int64) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET content_hash = 'edited' WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"a close", func(t *testing.T, ctx context.Context, tx pgx.Tx, id int64) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := startPostgres(t)
			ids := seedCovered(t, pool)
			id := ids["acme:2"]
			var first RefreshCoveredJobRow
			err := raceWith(t, pool, func(t *testing.T, ctx context.Context, tx pgx.Tx) { tc.hold(t, ctx, tx, id) },
				func(ctx context.Context, q *Queries) error {
					var err error
					first, err = q.RefreshCoveredJob(ctx, RefreshCoveredJobParams{Source: "greenhouse", ExternalID: "acme:2"})
					return err
				})
			if tc.name == "a close" {
				// The statement's snapshot saw the row open; the close moved the fence.
				if err != nil || first.Refreshed {
					t.Fatalf("first = %+v, %v; want refreshed=false (fenced)", first, err)
				}
				if _, err := New(pool).RefreshCoveredJob(context.Background(), RefreshCoveredJobParams{Source: "greenhouse", ExternalID: "acme:2"}); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatalf("fresh statement: err = %v, want pgx.ErrNoRows (now closed)", err)
				}
				return
			}
			if err != nil || first.Refreshed != tc.wantFirstOK {
				t.Fatalf("first = %+v, %v; want refreshed=%v", first, err, tc.wantFirstOK)
			}
			if !tc.wantFirstOK {
				second, err := New(pool).RefreshCoveredJob(context.Background(), RefreshCoveredJobParams{Source: "greenhouse", ExternalID: "acme:2"})
				if err != nil || !second.Refreshed {
					t.Fatalf("fresh statement = %+v, %v; want refreshed", second, err)
				}
			}
			if age := obsSeenAge(t, pool, id); age > time.Minute {
				t.Errorf("last_seen is %s old, want refreshed", age)
			}
		})
	}
}

// A writer that visits rows in DESC order in one transaction (the unsorted multi-row writers: the
// dedup and close passes) while a covered refresh runs: the bulk statement skips what the writer
// holds, the retries wait without holding anything, and nobody is chosen as a deadlock victim.
func TestCoveredRefreshNeverFormsADeadlockWithAnUnsortedWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := startPostgres(t)
	ids := seedCovered(t, pool)

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	touch := func(ext string) {
		t.Helper()
		if _, err := writer.Exec(ctx, `UPDATE jobs SET content_hash = 'w-' || id WHERE id = $1`, ids[ext]); err != nil {
			t.Fatalf("writer %s: %v", ext, err)
		}
	}
	touch("acme:5")
	touch("acme:4")

	rows, err := New(pool).RefreshCoveredJobs(ctx, coveredParams("acme:1", "acme:2", "acme:3", "acme:4", "acme:5"))
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	var unwritten []string
	for _, r := range rows {
		if !r.Refreshed {
			unwritten = append(unwritten, r.ExternalID)
		}
	}
	slices.Sort(unwritten)
	if !slices.Equal(unwritten, []string{"acme:4", "acme:5"}) {
		t.Fatalf("unwritten = %v, want exactly the rows the writer holds", unwritten)
	}

	// Retries wait for the writer while it still needs the rows below.
	results := make(chan error, 2)
	for _, ext := range unwritten {
		go func() {
			for attempt := 0; attempt < 3; attempt++ {
				r, err := New(pool).RefreshCoveredJob(ctx, RefreshCoveredJobParams{Source: "greenhouse", ExternalID: ext})
				if err != nil || r.Refreshed {
					results <- err
					return
				}
			}
			results <- errors.New("fenced three times")
		}()
	}
	waitForBlockedBackends(t, ctx, pool, 2)
	touch("acme:3")
	touch("acme:2")
	touch("acme:1")
	if err := writer.Commit(ctx); err != nil {
		t.Fatalf("writer commit (it must not be the deadlock victim): %v", err)
	}
	for range unwritten {
		if err := <-results; err != nil {
			t.Fatalf("retry: %v", err)
		}
	}
}

func waitForBlockedBackends(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	for {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("only %d backends blocked, want %d", n, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A cheap transaction holds the activity row and then inserts a child row (the FK takes KEY SHARE
// on the jobs tuple) while a full write holds the jobs tuple and waits for the activity row (its
// own state trigger). UpsertJob's DO UPDATE writes no unique-key column, so its tuple lock is
// NO KEY UPDATE, which KEY SHARE does not conflict with: the child insert goes through, the cheap
// transaction commits, the upsert finishes. Naming public_slug in the SET list would upgrade the
// lock and close a deadlock cycle.
func TestFullWriteDoesNotDeadlockWithACheapTransactionInsertingAChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := startPostgres(t)
	id := obsSeedStaleHashed(t, pool)

	cheap, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cheap.Rollback(context.Background()) }()
	if row, err := New(cheap).RefreshUnchangedJob(ctx, obsRefreshParams()); err != nil || row.Fenced {
		t.Fatalf("cheap refresh = %+v, %v", row, err)
	}

	full, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer full.Release()
	done := make(chan error, 1)
	go func() {
		p := obsHashedParams()
		p.ContentHash.String = "h2" // an edit: the full path, a state-trigger write to the activity row
		_, err := New(full).UpsertJob(ctx, p)
		done <- err
	}()
	obsWaitBlocked(t, ctx, pool, full.Conn().PgConn().PID(), done)

	if _, err := cheap.Exec(ctx, `INSERT INTO enrichment_outbox (job_id, target_version) VALUES ($1, 99)`, id); err != nil {
		t.Fatalf("child insert while the full write holds the jobs tuple: %v (a lock upgrade would deadlock here)", err)
	}
	if err := cheap.Commit(ctx); err != nil {
		t.Fatalf("cheap commit: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("full upsert: %v", err)
	}
}
