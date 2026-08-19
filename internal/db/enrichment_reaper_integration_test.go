//go:build integration

// Integration tests for the enrichment-queue reaper: deleting entries
// ClaimEnrichmentBatch can never take. The claim's inner join and its closed/duplicate
// filter correctly skip them; until this existed nothing deleted them either, and on prod
// they had grown into 57% of the queue.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

// TestDeleteIneligibleEnrichmentOutboxReapsWhatClaimCanNeverTake pins the reaper's
// boundary: it deletes exactly what the claim skips forever, and nothing the claim would
// still take.
func TestDeleteIneligibleEnrichmentOutboxReapsWhatClaimCanNeverTake(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	live := insertJob(t, pool, "live")
	canon := insertJob(t, pool, "canon")
	closedJob := insertJob(t, pool, "closed")
	repost := insertJob(t, pool, "repost")
	deadLettered := insertJob(t, pool, "dead")

	if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closedJob); err != nil {
		t.Fatalf("close job: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, canon, repost); err != nil {
		t.Fatalf("mark repost: %v", err)
	}
	// A dead-lettered entry whose job ALSO closed: ineligible on both counts, and it must
	// still survive. failed_at is the record of repeated failure that
	// freehire_queue_dead_letters reports, so reaping it would erase the evidence rather
	// than the garbage — the same rule the search reaper follows.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, deadLettered); err != nil {
		t.Fatalf("close dead-lettered job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE enrichment_outbox SET failed_at = now() WHERE job_id = $1`, deadLettered); err != nil {
		t.Fatalf("dead-letter entry: %v", err)
	}

	reaped, err := q.DeleteIneligibleEnrichmentOutbox(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteIneligibleEnrichmentOutbox: %v", err)
	}
	if reaped != 2 {
		t.Errorf("reaped %d entries, want 2 (the closed job's and the repost's)", reaped)
	}

	remaining := map[int64]bool{}
	rows, err := pool.Query(ctx, `SELECT job_id FROM enrichment_outbox`)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		remaining[id] = true
	}
	if !remaining[live] {
		t.Error("the live job's entry was reaped; the claim would still take it")
	}
	if !remaining[canon] {
		t.Error("the canonical job's entry was reaped; the claim would still take it")
	}
	if !remaining[deadLettered] {
		t.Error("the dead-lettered entry was reaped; failed_at is evidence, not garbage")
	}
	if remaining[closedJob] || remaining[repost] {
		t.Error("an ineligible entry survived; the claim skips it forever, so nothing else would remove it")
	}
}

// TestDeleteIneligibleEnrichmentOutboxRespectsItsBound guards the reason the query takes a
// limit at all: the backlog it first meets is measured in hundreds of thousands, and an
// unbounded delete would hold row locks the claim then waits on.
func TestDeleteIneligibleEnrichmentOutboxRespectsItsBound(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, name := range []string{"a", "b", "c"} {
		id := insertJob(t, pool, name)
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatalf("enqueue %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, id); err != nil {
			t.Fatalf("close %s: %v", name, err)
		}
	}

	first, err := q.DeleteIneligibleEnrichmentOutbox(ctx, 2)
	if err != nil {
		t.Fatalf("first reap: %v", err)
	}
	if first != 2 {
		t.Fatalf("first reap removed %d, want exactly the bound of 2", first)
	}
	second, err := q.DeleteIneligibleEnrichmentOutbox(ctx, 2)
	if err != nil {
		t.Fatalf("second reap: %v", err)
	}
	if second != 1 {
		t.Errorf("second reap removed %d, want the remaining 1", second)
	}
	third, err := q.DeleteIneligibleEnrichmentOutbox(ctx, 2)
	if err != nil {
		t.Fatalf("third reap: %v", err)
	}
	if third != 0 {
		t.Errorf("third reap removed %d, want 0 — the backlog is drained", third)
	}
}
