//go:build integration

// Lock order of the two observation writers a release A binary still runs, against the activity-first
// writers of release B. The global order is jobs, then job_activity, and ONLY THEN the queue
// (search_outbox): a writer that queues a row before it takes the activity row meets one that
// holds the activity row and queues, and each waits for the other. A release A statement used to
// take the activity row last, through the legacy bridge trigger at the end of the statement; it
// must lock jobs and job_activity explicitly, up front.
//
// The fixtures write job_activity only and the assertions use the methods' own results, so they
// hold whichever columns the writers keep.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queuedObservers are the statements that, on a posting search had withheld as unseen, observe it
// and queue its return to search: the cheap refresh of an unchanged posting and the touch.
func queuedObservers() []struct {
	name string
	run  func(ctx context.Context, c *pgxpool.Conn) error
} {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true}
	return []struct {
		name string
		run  func(ctx context.Context, c *pgxpool.Conn) error
	}{
		{"RefreshUnchangedJob", func(ctx context.Context, c *pgxpool.Conn) error {
			p := obsRefreshParams()
			p.WithheldIfSeenBefore = cutoff
			_, err := New(c).RefreshUnchangedJob(ctx, p)
			return err
		}},
		{"TouchJob", func(ctx context.Context, c *pgxpool.Conn) error {
			_, err := New(c).TouchJob(ctx, TouchJobParams{
				Source: "greenhouse", ExternalID: "acme:1", AllowReopen: true, WithheldIfSeenBefore: cutoff,
			})
			return err
		}},
	}
}

// seedWithheld stores one open, canonical posting whose activity row says it was last seen 30 days
// ago (so both observers queue its return to search) and starts from an empty queue.
func seedWithheld(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx := context.Background()
	truncate(t, pool)
	job, err := ingestUpsert(ctx, New(pool), obsHashedParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`UPDATE job_activity SET last_seen_at = now() - interval '30 days' WHERE job_id = $1`,
		`DELETE FROM search_outbox WHERE job_id = $1`,
	} {
		if _, err := pool.Exec(ctx, stmt, job.ID); err != nil {
			t.Fatal(err)
		}
	}
	return job.ID
}

// activityHeldByAnother reports whether some other transaction holds the activity row: a
// NO KEY UPDATE lock request that must not wait fails with lock_not_available (55P03).
func activityHeldByAnother(t *testing.T, pool *pgxpool.Pool, jobID int64) bool {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT 1 FROM job_activity WHERE job_id = $1 FOR NO KEY UPDATE NOWAIT`, jobID)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "55P03" {
		return true
	}
	if err != nil {
		t.Fatalf("probe the activity row: %v", err)
	}
	return false
}

// A writer that waits for the queue must already hold the activity row: that is what makes the
// queue the LAST lock of the order.
func TestObserversHoldTheActivityRowBeforeTheyWaitForTheQueue(t *testing.T) {
	for _, o := range queuedObservers() {
		t.Run(o.name, func(t *testing.T) {
			pool := startPostgres(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			id := seedWithheld(t, pool)

			// Another transaction owns the posting's queue entry, uncommitted.
			holder, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Rollback(context.Background()) }()
			if _, err := holder.Exec(ctx, `INSERT INTO search_outbox (job_id, job_posted_at) VALUES ($1, now())`, id); err != nil {
				t.Fatal(err)
			}
			c, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Release()
			done := make(chan error, 1)
			go func() { done <- o.run(ctx, c) }()
			obsWaitBlocked(t, ctx, pool, c.Conn().PgConn().PID(), done)

			if !activityHeldByAnother(t, pool, id) {
				t.Error("the observer is waiting for the queue without holding the activity row: an activity-first writer can queue now and deadlock with it")
			}
			if err := holder.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("%s: %v", o.name, err)
			}
		})
	}
}

// The deadlock itself: a writer that takes the activity row and then queues, interleaved with the
// observer. Whichever finishes first, neither is chosen as a deadlock victim.
func TestObserversDoNotDeadlockWithAnActivityFirstWriter(t *testing.T) {
	for _, o := range queuedObservers() {
		t.Run(o.name, func(t *testing.T) {
			pool := startPostgres(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			id := seedWithheld(t, pool)

			// The activity-first writer: activity row, then the queue.
			writer, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = writer.Rollback(context.Background()) }()
			if _, err := writer.Exec(ctx, `UPDATE job_activity SET last_seen_at = last_seen_at WHERE job_id = $1`, id); err != nil {
				t.Fatal(err)
			}
			c, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Release()
			done := make(chan error, 1)
			go func() { done <- o.run(ctx, c) }()
			obsWaitBlocked(t, ctx, pool, c.Conn().PgConn().PID(), done)

			_, werr := writer.Exec(ctx, `INSERT INTO search_outbox (job_id, job_posted_at) VALUES ($1, now())
				ON CONFLICT (job_id) DO UPDATE SET id = DEFAULT, created_at = now(), attempts = 0`, id)
			if werr == nil {
				werr = writer.Commit(ctx)
			} else {
				_ = writer.Rollback(ctx)
			}
			oerr := <-done
			if werr != nil || oerr != nil {
				t.Fatalf("activity-first writer: %v; %s: %v (a 40P01 here is the lock-order inversion)", werr, o.name, oerr)
			}
		})
	}
}
