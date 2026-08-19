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
	"fmt"
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

// TestEnqueueSearchOutboxDenormalizesJobPostedAt covers the fix for ClaimSearchOutboxBatch's
// join-for-ordering cost (see that query's doc comment): EnqueueSearchOutbox must stamp
// job_posted_at from COALESCE(jobs.posted_at, jobs.created_at) at enqueue time so the claim
// query can sort without joining jobs.
func TestEnqueueSearchOutboxDenormalizesJobPostedAt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	withPostedAt := ingestParams("acme:posted", "Has PostedAt")
	postedAt := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	withPostedAt.PostedAt = pgtype.Timestamptz{Time: postedAt, Valid: true}
	job, err := ingestUpsert(ctx, q, withPostedAt)
	if err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	if err := q.EnqueueSearchOutbox(ctx, job.ID); err != nil {
		t.Fatalf("enqueue job: %v", err)
	}

	got := jobPostedAtOf(ctx, t, pool, job.ID)
	if !got.Valid || !got.Time.Equal(postedAt) {
		t.Fatalf("job_posted_at = %+v, want %v", got, postedAt)
	}

	// No posted_at at all: falls back to jobs.created_at, mirroring the query's COALESCE.
	noPostedAt, err := ingestUpsert(ctx, q, ingestParams("acme:no-posted", "No PostedAt"))
	if err != nil {
		t.Fatalf("upsert job without posted_at: %v", err)
	}
	if err := q.EnqueueSearchOutbox(ctx, noPostedAt.ID); err != nil {
		t.Fatalf("enqueue job without posted_at: %v", err)
	}
	got = jobPostedAtOf(ctx, t, pool, noPostedAt.ID)
	if !got.Valid || !got.Time.Equal(noPostedAt.CreatedAt.Time) {
		t.Fatalf("job_posted_at = %+v, want fallback to created_at %v", got, noPostedAt.CreatedAt)
	}
}

// TestClaimSearchOutboxBatchOrdersByJobPostedAtAndSkipsClosedOrDuplicate covers
// ClaimSearchOutboxBatch's two load-bearing properties after moving the sort onto the
// outbox's own denormalized job_posted_at column: it still returns freshest-job-first, and
// it still skips (without claiming) a job that closed or became a non-canonical repost
// after being queued — the same behavior the old jobs-join version had, now reached via an
// index scan plus a per-row EXISTS check instead of a join-then-sort.
func TestDeleteIneligibleSearchOutboxReapsWhatClaimCanNeverTake(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	open, err := ingestUpsert(ctx, q, ingestParams("acme:open", "Open"))
	if err != nil {
		t.Fatalf("upsert open: %v", err)
	}
	canon, err := ingestUpsert(ctx, q, ingestParams("acme:canon", "Canon"))
	if err != nil {
		t.Fatalf("upsert canon: %v", err)
	}
	closedJob, err := ingestUpsert(ctx, q, ingestParams("acme:closed", "Closed"))
	if err != nil {
		t.Fatalf("upsert closed: %v", err)
	}
	repost, err := ingestUpsert(ctx, q, ingestParams("acme:repost", "Repost"))
	if err != nil {
		t.Fatalf("upsert repost: %v", err)
	}
	deadLettered, err := ingestUpsert(ctx, q, ingestParams("acme:dead", "Dead"))
	if err != nil {
		t.Fatalf("upsert dead: %v", err)
	}

	for _, id := range []int64{open.ID, canon.ID, closedJob.ID, repost.ID, deadLettered.ID} {
		if err := q.EnqueueSearchOutbox(ctx, id); err != nil {
			t.Fatalf("enqueue job %d: %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closedJob.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, canon.ID, repost.ID); err != nil {
		t.Fatalf("mark repost as duplicate: %v", err)
	}
	// A dead-lettered entry whose job ALSO closed: it is ineligible on both counts, and
	// must still survive — failed_at is the record of repeated failure that
	// freehire_queue_dead_letters reports, so reaping it would erase the evidence
	// rather than the garbage.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now() WHERE id = $1`, deadLettered.ID); err != nil {
		t.Fatalf("close dead-lettered job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE search_outbox SET failed_at = now() WHERE job_id = $1`, deadLettered.ID); err != nil {
		t.Fatalf("dead-letter entry: %v", err)
	}

	reaped, err := q.DeleteIneligibleSearchOutbox(ctx, 100)
	if err != nil {
		t.Fatalf("DeleteIneligibleSearchOutbox: %v", err)
	}
	if reaped != 2 {
		t.Errorf("reaped %d rows, want 2 (the closed job's and the repost's)", reaped)
	}

	var surviving []int64
	rows, err := pool.Query(ctx, `SELECT job_id FROM search_outbox ORDER BY job_id`)
	if err != nil {
		t.Fatalf("list surviving: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		surviving = append(surviving, id)
	}

	for _, id := range []int64{open.ID, canon.ID, deadLettered.ID} {
		if !containsInt64(surviving, id) {
			t.Errorf("job %d's entry was reaped; only entries claim can never take may go", id)
		}
	}
	for _, id := range []int64{closedJob.ID, repost.ID} {
		if containsInt64(surviving, id) {
			t.Errorf("job %d's entry survived; claim skips it forever, so nothing else would ever remove it", id)
		}
	}
}

func TestDeleteIneligibleSearchOutboxRespectsItsBound(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Five reapable entries, reaped two at a time: one drain run must not turn into an
	// unbounded delete over a backlog that accumulated for weeks.
	for i := range 5 {
		job, err := ingestUpsert(ctx, q, ingestParams(fmt.Sprintf("acme:closed-%d", i), "Closed"))
		if err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
		if err := q.EnqueueSearchOutbox(ctx, job.ID); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, job.ID); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
	}

	first, err := q.DeleteIneligibleSearchOutbox(ctx, 2)
	if err != nil {
		t.Fatalf("first reap: %v", err)
	}
	if first != 2 {
		t.Errorf("first reap removed %d rows, want exactly the 2 it was bounded to", first)
	}

	var left int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_outbox`).Scan(&left); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if left != 3 {
		t.Errorf("%d entries left, want 3 — the next run takes the next slice", left)
	}
}

func TestClaimSearchOutboxBatchOrdersByJobPostedAtAndSkipsClosedOrDuplicate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	older, err := ingestUpsert(ctx, q, ingestParams("acme:older", "Older"))
	if err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	newer, err := ingestUpsert(ctx, q, ingestParams("acme:newer", "Newer"))
	if err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	closedJob, err := ingestUpsert(ctx, q, ingestParams("acme:closed", "Closed"))
	if err != nil {
		t.Fatalf("upsert closed: %v", err)
	}
	canon, err := ingestUpsert(ctx, q, ingestParams("acme:canon", "Canon"))
	if err != nil {
		t.Fatalf("upsert canon: %v", err)
	}
	repost, err := ingestUpsert(ctx, q, ingestParams("acme:repost", "Repost"))
	if err != nil {
		t.Fatalf("upsert repost: %v", err)
	}

	for _, id := range []int64{older.ID, newer.ID, closedJob.ID, canon.ID, repost.ID} {
		if err := q.EnqueueSearchOutbox(ctx, id); err != nil {
			t.Fatalf("enqueue job %d: %v", id, err)
		}
	}

	// Order the three live, open, canonical entries by job_posted_at.
	olderAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	canonAt := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	newerAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET job_posted_at = $1 WHERE job_id = $2`, olderAt, older.ID); err != nil {
		t.Fatalf("pin older job_posted_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET job_posted_at = $1 WHERE job_id = $2`, canonAt, canon.ID); err != nil {
		t.Fatalf("pin canon job_posted_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE search_outbox SET job_posted_at = $1 WHERE job_id = $2`, newerAt, newer.ID); err != nil {
		t.Fatalf("pin newer job_posted_at: %v", err)
	}

	// closedJob's outbox entry stays queued, but the job itself is now closed.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closedJob.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}
	// repost's outbox entry stays queued, but the job itself became a non-canonical
	// repost of canon after being enqueued.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, canon.ID, repost.ID); err != nil {
		t.Fatalf("mark repost as duplicate: %v", err)
	}

	// BatchSize smaller than the claimable set (older/canon/newer) so the ordering
	// actually determines which entries make the cut, not just which get excluded.
	rows, err := q.ClaimSearchOutboxBatch(ctx, ClaimSearchOutboxBatchParams{LeaseSeconds: 180, BatchSize: 2})
	if err != nil {
		t.Fatalf("ClaimSearchOutboxBatch: %v", err)
	}

	var claimedJobIDs []int64
	for _, r := range rows {
		claimedJobIDs = append(claimedJobIDs, r.JobID)
	}
	want := []int64{newer.ID, canon.ID}
	if len(claimedJobIDs) != 2 {
		t.Fatalf("claimed job_ids = %v, want exactly 2 entries (newer and canon)", claimedJobIDs)
	}
	if containsInt64(claimedJobIDs, closedJob.ID) {
		t.Fatalf("claimed job_ids = %v, want closed job %d excluded", claimedJobIDs, closedJob.ID)
	}
	if containsInt64(claimedJobIDs, repost.ID) {
		t.Fatalf("claimed job_ids = %v, want non-canonical repost %d excluded", claimedJobIDs, repost.ID)
	}
	if !containsInt64(claimedJobIDs, newer.ID) || !containsInt64(claimedJobIDs, canon.ID) {
		t.Fatalf("claimed job_ids = %v, want %v", claimedJobIDs, want)
	}
	if containsInt64(claimedJobIDs, older.ID) {
		t.Fatalf("claimed job_ids = %v, want older job %d NOT claimed (batch is full before it sorts in)", claimedJobIDs, older.ID)
	}

	// The two skipped entries (closed, repost) remain unclaimed — claimed_at stays NULL —
	// so something else has to clear them. That used to be assumed to be the full
	// reindex; it was not (see DeleteIneligibleSearchOutbox), and they accumulated for
	// weeks. The drain now reaps them.
	for _, id := range []int64{closedJob.ID, repost.ID} {
		var claimedAt pgtype.Timestamptz
		if err := pool.QueryRow(ctx, `SELECT claimed_at FROM search_outbox WHERE job_id = $1`, id).Scan(&claimedAt); err != nil {
			t.Fatalf("query claimed_at for job %d: %v", id, err)
		}
		if claimedAt.Valid {
			t.Fatalf("job %d's outbox entry was claimed, want it left unclaimed", id)
		}
	}
}

func jobPostedAtOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, jobID int64) pgtype.Timestamptz {
	t.Helper()
	var v pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT job_posted_at FROM search_outbox WHERE job_id = $1`, jobID).Scan(&v); err != nil {
		t.Fatalf("query job_posted_at for job %d: %v", jobID, err)
	}
	return v
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
