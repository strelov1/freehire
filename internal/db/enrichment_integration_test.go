//go:build integration

// Integration tests for the enrichment_outbox queue semantics — claim/lease,
// idempotent enqueue, and dead-lettering — which are SQL behavior and can only be
// verified against a real Postgres. Run with: go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

const targetVersion int32 = 1

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// insertJob inserts a job with is_tech = true and a non-empty description (the
// enrichment enqueue gate's default requirements — see TestEnqueueGatesOnIsTech and
// TestEnqueueGatesOnDescription for the gating itself), so every other test in this
// file that enqueues and expects it to succeed doesn't have to set either.
func insertJob(t *testing.T, pool *pgxpool.Pool, externalID string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, description, public_slug, is_tech)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'Build things.', 'job-' || $1, true) RETURNING id`,
		externalID).Scan(&id)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE enrichment_outbox, jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// setPostedAt stamps a job's posted_at so claim-ordering tests can control freshness.
func setPostedAt(t *testing.T, pool *pgxpool.Pool, jobID int64, posted string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET posted_at = $1 WHERE id = $2", posted, jobID); err != nil {
		t.Fatalf("set posted_at: %v", err)
	}
}

// setCategory stamps a job's derived category so the non-tech enqueue gate can be tested.
func setCategory(t *testing.T, pool *pgxpool.Pool, jobID int64, category string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET category = $1 WHERE id = $2", category, jobID); err != nil {
		t.Fatalf("set category: %v", err)
	}
}

// setIsTech stamps a job's tri-state is_tech signal (nil clears it to SQL NULL) so the
// enrichment enqueue gate can be tested against all three states.
func setIsTech(t *testing.T, pool *pgxpool.Pool, jobID int64, isTech *bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET is_tech = $1 WHERE id = $2", isTech, jobID); err != nil {
		t.Fatalf("set is_tech: %v", err)
	}
}

// setDescription stamps a job's description so the enrichment enqueue gate's
// description <> ” condition can be tested.
func setDescription(t *testing.T, pool *pgxpool.Pool, jobID int64, description string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET description = $1 WHERE id = $2", description, jobID); err != nil {
		t.Fatalf("set description: %v", err)
	}
}

// enqueuedJobIDs returns the outbox's job_ids in ascending order for assertions.
func enqueuedJobIDs(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT job_id FROM enrichment_outbox ORDER BY job_id")
	if err != nil {
		t.Fatalf("select outbox: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan job_id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

// closeJob soft-closes a job (sets closed_at) so claim/enqueue exclusion can be tested.
func closeJob(t *testing.T, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET closed_at = now() WHERE id = $1", jobID); err != nil {
		t.Fatalf("close job: %v", err)
	}
}

func TestEnrichmentClaimPriority(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("fresher open jobs are claimed first", func(t *testing.T) {
		truncate(t, pool)
		// Insert the older-posted job first so its outbox id is lower than the newer
		// job's — proving the claim orders by posted_at, not insertion id.
		older := insertJob(t, pool, "older")
		newer := insertJob(t, pool, "newer")
		setPostedAt(t, pool, older, "2024-01-01T00:00:00Z")
		setPostedAt(t, pool, newer, "2024-06-01T00:00:00Z")
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}

		claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 2 {
			t.Fatalf("claim: rows=%d err=%v, want 2", len(claimed), err)
		}
		if claimed[0].JobID != newer || claimed[1].JobID != older {
			t.Errorf("claim order = [%d, %d], want [%d, %d] (newer posted_at first)",
				claimed[0].JobID, claimed[1].JobID, newer, older)
		}
	})

	t.Run("undated jobs rank by created_at, not last", func(t *testing.T) {
		truncate(t, pool)
		// An old dated job vs a job with no posted_at but freshly ingested. Under
		// NULLS LAST the undated one would sort last; COALESCE(posted_at, created_at)
		// ranks it by its (recent) created_at, so it is claimed first.
		dated := insertJob(t, pool, "dated")
		setPostedAt(t, pool, dated, "2024-01-01T00:00:00Z")
		undated := insertJob(t, pool, "undated") // posted_at NULL, created_at = now()
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}

		claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 2 {
			t.Fatalf("claim: rows=%d err=%v, want 2", len(claimed), err)
		}
		if claimed[0].JobID != undated || claimed[1].JobID != dated {
			t.Errorf("claim order = [%d, %d], want [%d, %d] (undated-but-recent first)",
				claimed[0].JobID, claimed[1].JobID, undated, dated)
		}
	})

	t.Run("closed jobs are not enqueued", func(t *testing.T) {
		truncate(t, pool)
		open := insertJob(t, pool, "open")
		gone := insertJob(t, pool, "closed")
		closeJob(t, pool, gone)
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}

		var jobIDs []int64
		rows, err := pool.Query(ctx, "SELECT job_id FROM enrichment_outbox ORDER BY job_id")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			jobIDs = append(jobIDs, id)
		}
		if len(jobIDs) != 1 || jobIDs[0] != open {
			t.Errorf("enqueued job_ids = %v, want only the open job %d", jobIDs, open)
		}
	})

	t.Run("entries for closed jobs are not claimed", func(t *testing.T) {
		truncate(t, pool)
		open := insertJob(t, pool, "open")
		gone := insertJob(t, pool, "gone")
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}
		// Close one job after it was queued: the claim-time filter must skip it.
		closeJob(t, pool, gone)

		claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if len(claimed) != 1 || claimed[0].JobID != open {
			t.Errorf("claimed = %+v, want only the open job %d", claimed, open)
		}
	})
}

func TestEnrichmentQueue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("enqueue is idempotent", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "idem")

		for i := 0; i < 2; i++ {
			if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
				t.Fatalf("enqueue: %v", err)
			}
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM enrichment_outbox").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("outbox rows = %d, want 1 (one per (job_id, target_version))", n)
		}
	})

	t.Run("claim leases entries so concurrent claims are disjoint", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "j1")
		insertJob(t, pool, "j2")
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}

		first, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 1})
		if err != nil || len(first) != 1 {
			t.Fatalf("first claim: rows=%d err=%v, want 1", len(first), err)
		}
		second, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(second) != 1 {
			t.Fatalf("second claim: rows=%d err=%v, want 1 (the other entry)", len(second), err)
		}
		if first[0].ID == second[0].ID {
			t.Errorf("both claims returned outbox id %d — not disjoint", first[0].ID)
		}
		third, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(third) != 0 {
			t.Errorf("third claim: rows=%d, want 0 (all leased)", len(third))
		}
	})

	t.Run("a stale lease is reclaimable", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "stale")
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}

		if c, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(c), err)
		}
		// Still within the lease → not reclaimable.
		if c, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Fatalf("re-claim within lease: rows=%d, want 0", len(c))
		}
		// Lease of 0s → the prior claim is now stale and reclaimable.
		if c, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Errorf("re-claim with expired lease: rows=%d err=%v, want 1", len(c), err)
		}
	})

	t.Run("attempts reaching max dead-letters the entry", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "dead")
		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(claimed), err)
		}
		id := claimed[0].ID

		// PostingAtFault: the attempt ceiling only governs failures the posting caused.
		first, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
			LastError: "boom", MaxAttempts: 2, PostingAtFault: true, UpstreamGraceDays: 14, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if first.Attempts != 1 || first.FailedAt.Valid {
			t.Errorf("after 1st failure: attempts=%d failed=%v, want 1/not-dead", first.Attempts, first.FailedAt.Valid)
		}
		second, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
			LastError: "boom", MaxAttempts: 2, PostingAtFault: true, UpstreamGraceDays: 14, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if second.Attempts != 2 || !second.FailedAt.Valid {
			t.Errorf("after 2nd failure: attempts=%d failed=%v, want 2/dead-lettered", second.Attempts, second.FailedAt.Valid)
		}
		// Dead-lettered → never claimed again, even with an expired lease.
		if c, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Errorf("claim after dead-letter: rows=%d, want 0", len(c))
		}
	})
}

// TestEnqueueGatesOnIsTech covers the AI-budget gate: both enqueue paths enqueue a job
// only when its already-derived tri-state is_tech is TRUE — never a confirmed non-tech
// job (is_tech = false) and, deliberately, never one the title dictionary and
// description could place in neither direction either (is_tech IS NULL). See
// EnqueueJobEnrichment's doc comment for why the NULL case is excluded too, not just
// false: at catalogue scale it was ~65% of the open catalogue and enrichment found
// nothing useful for ~91% of it.
func TestEnqueueGatesOnIsTech(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	trueVal, falseVal := true, false

	t.Run("backfill enqueue skips false and NULL, keeps true", func(t *testing.T) {
		truncate(t, pool)
		tech := insertJob(t, pool, "tech")
		setIsTech(t, pool, tech, &trueVal)
		nonTech := insertJob(t, pool, "nontech")
		setIsTech(t, pool, nonTech, &falseVal)
		unresolved := insertJob(t, pool, "unresolved")
		setIsTech(t, pool, unresolved, nil)

		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}

		got := enqueuedJobIDs(t, pool)
		if len(got) != 1 || got[0] != tech {
			t.Fatalf("enqueued = %v, want [%d] (only the tech job)", got, tech)
		}
	})

	t.Run("transactional enqueue skips a confirmed non-tech job", func(t *testing.T) {
		truncate(t, pool)
		mgmt := insertJob(t, pool, "mgmt")
		setIsTech(t, pool, mgmt, &falseVal)

		n, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{
			TargetVersion: targetVersion,
			JobID:         mgmt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("enqueued rows = %d, want 0 for a confirmed non-tech job", n)
		}
		if got := enqueuedJobIDs(t, pool); len(got) != 0 {
			t.Errorf("outbox = %v, want empty", got)
		}
	})

	t.Run("transactional enqueue skips an unresolved job", func(t *testing.T) {
		truncate(t, pool)
		unresolved := insertJob(t, pool, "unresolved")
		setIsTech(t, pool, unresolved, nil)

		n, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{
			TargetVersion: targetVersion,
			JobID:         unresolved,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("enqueued rows = %d, want 0 for an is_tech-unresolved job", n)
		}
		if got := enqueuedJobIDs(t, pool); len(got) != 0 {
			t.Errorf("outbox = %v, want empty", got)
		}
	})
}

// TestEnqueueGatesOnDescription covers the other half of the AI-budget gate: a job
// with no description is never enqueued, whatever its is_tech state — the LLM has
// nothing to extract from a blank one. A 2026-08-06 prod sweep found ~53K such rows
// already queued for no reason (see EnqueueJobEnrichment's doc comment).
func TestEnqueueGatesOnDescription(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("backfill enqueue skips a tech job with a blank description", func(t *testing.T) {
		truncate(t, pool)
		blank := insertJob(t, pool, "blank")
		setDescription(t, pool, blank, "")

		if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
			t.Fatal(err)
		}
		if got := enqueuedJobIDs(t, pool); len(got) != 0 {
			t.Errorf("enqueued = %v, want none (blank description)", got)
		}
	})

	t.Run("transactional enqueue skips a tech job with a blank description", func(t *testing.T) {
		truncate(t, pool)
		blank := insertJob(t, pool, "blank")
		setDescription(t, pool, blank, "")

		n, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{
			TargetVersion: targetVersion,
			JobID:         blank,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("enqueued rows = %d, want 0 for a blank-description job", n)
		}
		if got := enqueuedJobIDs(t, pool); len(got) != 0 {
			t.Errorf("outbox = %v, want empty", got)
		}
	})
}
