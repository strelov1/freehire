//go:build integration

// Integration test for DeleteSearchOutboxCreatedBefore: a full facet reindex reads
// every job's CURRENT content directly from Postgres, so any search_outbox row queued
// before the run started is provably already reflected in the freshly-built index.
// The purge must delete only those rows and leave rows queued at or after the cutoff
// alone — those may still represent a job changed after the reindex's scan passed it.
// Verifiable only against real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeleteSearchOutboxCreatedBefore(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	stale, err := ingestUpsert(ctx, q, ingestParams("acme:stale", "Stale"))
	if err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	atCutoff, err := ingestUpsert(ctx, q, ingestParams("acme:at-cutoff", "At Cutoff"))
	if err != nil {
		t.Fatalf("upsert at-cutoff: %v", err)
	}
	fresh, err := ingestUpsert(ctx, q, ingestParams("acme:fresh", "Fresh"))
	if err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}

	if err := q.EnqueueSearchOutbox(ctx, stale.ID); err != nil {
		t.Fatalf("enqueue stale: %v", err)
	}
	if err := q.EnqueueSearchOutbox(ctx, atCutoff.ID); err != nil {
		t.Fatalf("enqueue at-cutoff: %v", err)
	}
	if err := q.EnqueueSearchOutbox(ctx, fresh.ID); err != nil {
		t.Fatalf("enqueue fresh: %v", err)
	}

	// Pin created_at (and, for the row expected to be purged, the job's own
	// updated_at too — the query requires both to predate the cutoff, see
	// DeleteSearchOutboxCreatedBefore's doc comment) to controlled instants:
	// EnqueueSearchOutbox and ingestUpsert both stamp now(), which would put every
	// row on the same side of any cutoff.
	before := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET created_at = $1 WHERE job_id = $2`, before, stale.ID); err != nil {
		t.Fatalf("pin stale created_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = $1 WHERE id = $2`, before, stale.ID); err != nil {
		t.Fatalf("pin stale job's updated_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET created_at = $1 WHERE job_id = $2`, cutoff, atCutoff.ID); err != nil {
		t.Fatalf("pin at-cutoff created_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET created_at = $1 WHERE job_id = $2`, after, fresh.ID); err != nil {
		t.Fatalf("pin fresh created_at: %v", err)
	}

	n, err := q.DeleteSearchOutboxCreatedBefore(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		t.Fatalf("DeleteSearchOutboxCreatedBefore: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d rows, want exactly 1 (the strictly-before-cutoff row)", n)
	}

	remaining := remainingOutboxJobIDs(ctx, t, pool)
	if len(remaining) != 2 || !containsInt64(remaining, atCutoff.ID) || !containsInt64(remaining, fresh.ID) {
		t.Fatalf("remaining outbox job_ids = %v, want exactly [%d %d]", remaining, atCutoff.ID, fresh.ID)
	}
	if containsInt64(remaining, stale.ID) {
		t.Fatalf("stale job %d's outbox row survived the purge", stale.ID)
	}
}

// TestDeleteSearchOutboxCreatedBefore_SurvivesRepeatChangeUnderConflictDoNothing covers
// the gap EnqueueSearchOutbox's ON CONFLICT (job_id) DO NOTHING opens: created_at is
// stamped only on a job's FIRST enqueue since its last drain, so a job re-changed while
// its outbox row is still pending keeps the OLD created_at. If the purge trusted
// created_at alone, a job modified again during the reindex's own scan window — after
// the scan already read its (now stale) row — would have its still-needed outbox entry
// deleted right along with the genuinely-redundant ones. The purge must also check the
// job's own updated_at (stamped in the same transaction as EnqueueSearchOutbox, per
// cmd/ingest/store.go) to catch this.
func TestDeleteSearchOutboxCreatedBefore_SurvivesRepeatChangeUnderConflictDoNothing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:repeat-change", "Repeat Change"))
	if err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	if err := q.EnqueueSearchOutbox(ctx, job.ID); err != nil {
		t.Fatalf("enqueue job: %v", err)
	}

	firstEnqueue := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	reindexStart := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	changedDuringRun := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET created_at = $1 WHERE job_id = $2`, firstEnqueue, job.ID); err != nil {
		t.Fatalf("pin outbox created_at to the first enqueue: %v", err)
	}
	// Simulates the job changing again while its outbox row is still pending: a real
	// second EnqueueSearchOutbox call would leave created_at untouched (ON CONFLICT DO
	// NOTHING), which is exactly what NOT re-pinning search_outbox.created_at here
	// reproduces — only jobs.updated_at moves.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = $1 WHERE id = $2`, changedDuringRun, job.ID); err != nil {
		t.Fatalf("pin job updated_at to the repeat change: %v", err)
	}

	n, err := q.DeleteSearchOutboxCreatedBefore(ctx, pgtype.Timestamptz{Time: reindexStart, Valid: true})
	if err != nil {
		t.Fatalf("DeleteSearchOutboxCreatedBefore: %v", err)
	}
	if n != 0 {
		t.Fatalf("purged %d rows, want 0 — the job changed again after reindexStart and still needs draining", n)
	}

	remaining := remainingOutboxJobIDs(ctx, t, pool)
	if !containsInt64(remaining, job.ID) {
		t.Fatalf("job %d's outbox row was purged despite a repeat change during the run", job.ID)
	}
}

func remainingOutboxJobIDs(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT job_id FROM search_outbox ORDER BY job_id`)
	if err != nil {
		t.Fatalf("query remaining outbox rows: %v", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var jobID int64
		if err := rows.Scan(&jobID); err != nil {
			t.Fatalf("scan job_id: %v", err)
		}
		out = append(out, jobID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func containsInt64(xs []int64, want int64) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
