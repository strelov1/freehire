//go:build integration

// Integration tests for the enrichment-queue reaper: deleting entries
// ClaimEnrichmentBatch can never take. The claim's inner join and its closed/duplicate
// filter correctly skip them; until this existed nothing deleted them either, and on prod
// they had grown into 57% of the queue.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

// reapEnrichment runs DeleteIneligibleEnrichmentOutbox over the whole queue the way cmd/enrich
// does, chunk entries at a time, deleting at most maxRows.
func reapEnrichment(t *testing.T, q *Queries, chunk, maxRows int32) int64 {
	t.Helper()
	var reaped int64
	for after := int64(0); reaped < int64(maxRows); {
		r, err := q.DeleteIneligibleEnrichmentOutbox(context.Background(), DeleteIneligibleEnrichmentOutboxParams{
			AfterJobID: after, ChunkSize: chunk, MaxRows: maxRows - int32(reaped),
		})
		if err != nil {
			t.Fatalf("DeleteIneligibleEnrichmentOutbox after job %d: %v", after, err)
		}
		reaped += r.Deleted
		if r.LastJobID == 0 {
			break
		}
		after = r.LastJobID
	}
	return reaped
}

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

	if _, err := q.EnqueuePendingJobs(ctx, EnqueuePendingJobsParams{TargetVersion: targetVersion}); err != nil {
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

	// The closed job also has an entry at a second version, so with one entry per chunk its
	// two entries straddle a chunk edge: both must go all the same.
	if _, err := pool.Exec(ctx,
		`INSERT INTO enrichment_outbox (job_id, target_version) VALUES ($1, $2)`, closedJob, targetVersion+1); err != nil {
		t.Fatalf("second version: %v", err)
	}

	if reaped := reapEnrichment(t, q, 1, 100); reaped != 3 {
		t.Errorf("reaped %d entries, want 3 (the closed job's two and the repost's)", reaped)
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
		if _, err := q.EnqueuePendingJobs(ctx, EnqueuePendingJobsParams{TargetVersion: targetVersion}); err != nil {
			t.Fatalf("enqueue %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, id); err != nil {
			t.Fatalf("close %s: %v", name, err)
		}
	}

	if first := reapEnrichment(t, q, 100, 2); first != 2 {
		t.Fatalf("first reap removed %d, want exactly the bound of 2", first)
	}
	if second := reapEnrichment(t, q, 100, 2); second != 1 {
		t.Errorf("second reap removed %d, want the remaining 1", second)
	}
	if third := reapEnrichment(t, q, 100, 2); third != 0 {
		t.Errorf("third reap removed %d, want 0 — the backlog is drained", third)
	}
}
