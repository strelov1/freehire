//go:build integration

// The full TouchJob and the ordinary touch (TouchOpenJob) on one posting that search had withheld
// for going unseen. Both queue it back through search_outbox ON CONFLICT DO UPDATE on job_id. The
// ordinary touch takes the activity row first and the outbox entry second; so the full touch must
// take the activity row BEFORE it writes the outbox entry (jobs, then job_activity, then the
// queues). Otherwise the full touch holds the entry while the ordinary touch holds the activity row
// and waits for the entry, and the full touch's ObserveJobActivity, its second command, waits for
// the activity row: a deadlock (SQLSTATE 40P01) that aborts one of two touches.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestFullTouchLocksTheActivityRowBeforeQueueingSearchWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := startPostgres(t)
	truncate(t, pool)
	q := New(pool)

	p := ingestParams("jj:withheld", "Job")
	p.Source = "justjoin"
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	// Last seen 30 days ago: both touches would queue the posting back into search.
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '30 days' WHERE job_id = $1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE search_outbox`); err != nil {
		t.Fatal(err)
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true}

	full, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = full.Rollback(context.Background()) }()
	row, err := New(full).TouchJob(ctx, TouchJobParams{Source: "justjoin", ExternalID: "jj:withheld", AllowReopen: true, WithheldIfSeenBefore: cutoff})
	if err != nil || row.Deferred {
		t.Fatalf("full touch = %+v, %v", row, err)
	}

	// The statement must already hold the activity row: another session cannot take it.
	probe, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, probeErr := probe.Exec(ctx, `SELECT 1 FROM job_activity WHERE job_id = $1 FOR NO KEY UPDATE NOWAIT`, job.ID)
	probe.Release()
	var pgErr *pgconn.PgError
	if !errors.As(probeErr, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("after the full touch statement the activity row is free (probe err = %v): it queued search work before locking job_activity", probeErr)
	}

	// The ordinary touch of the same posting waits for the activity row, not for the outbox entry.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	done := make(chan error, 1)
	go func() {
		_, err := New(conn).TouchOpenJob(ctx, TouchOpenJobParams{Source: "justjoin", ExternalID: "jj:withheld", WithheldIfSeenBefore: cutoff})
		done <- err
	}()
	obsWaitBlocked(t, ctx, pool, conn.Conn().PgConn().PID(), done)

	// The full touch's second command must not wait for anything the ordinary touch holds.
	if n, err := New(full).ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: row.ID}); err != nil || n != 1 {
		t.Fatalf("observe after the full touch = %d, %v (a 40P01 here is the lock-order inversion)", n, err)
	}
	if err := full.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// The ordinary touch then runs on the committed state: it was fenced or refreshed, never a deadlock.
	if err := <-done; err != nil {
		t.Fatalf("ordinary touch: %v", err)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_outbox WHERE job_id = $1`, job.ID).Scan(&queued); err != nil || queued != 1 {
		t.Errorf("search_outbox entries for the posting = %d (%v), want exactly 1", queued, err)
	}
}
