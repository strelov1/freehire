//go:build integration

// Integration tests for the semantic_outbox queue semantics — enqueue (missing /
// content-stale / model-stale / non-tech exclusion / closed-removal), claim/lease
// (which, unlike enrichment, must NOT drop closed jobs), the provenance stamp/clear,
// and dead-lettering. These are SQL behavior and can only be verified against a real
// Postgres. Run with: go test -tags=integration ./internal/db/ (requires Docker).
// Reuses the helpers in enrichment_integration_test.go (startPostgres, insertJob,
// truncate, setPostedAt, setCategory, closeJob) — truncate's `jobs ... CASCADE` also
// clears semantic_outbox via its FK.
package db

import (
	"context"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const targetModel = "e5-test-v1"

// setContentHash stamps a job's content_hash (the incremental-index change signal the
// enqueue keys staleness on).
func setContentHash(t *testing.T, pool *pgxpool.Pool, jobID int64, hash string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET content_hash = $1 WHERE id = $2", hash, jobID); err != nil {
		t.Fatalf("set content_hash: %v", err)
	}
}

// setSemanticStamp records a prior embed of a job under a model + content hash.
func setSemanticStamp(t *testing.T, pool *pgxpool.Pool, jobID int64, model, hash string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET semantic_embedded_model = $1, semantic_embedded_hash = $2 WHERE id = $3",
		model, hash, jobID); err != nil {
		t.Fatalf("set semantic stamp: %v", err)
	}
}

func semanticOutboxJobIDs(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT job_id FROM semantic_outbox ORDER BY job_id")
	if err != nil {
		t.Fatalf("select semantic_outbox: %v", err)
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

func semanticStamp(t *testing.T, pool *pgxpool.Pool, jobID int64) (model, hash *string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		"SELECT semantic_embedded_model, semantic_embedded_hash FROM jobs WHERE id = $1", jobID).
		Scan(&model, &hash); err != nil {
		t.Fatalf("read semantic stamp: %v", err)
	}
	return model, hash
}

func TestSemanticEnqueue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	enqueue := func(t *testing.T) {
		t.Helper()
		if _, err := q.EnqueuePendingSemanticJobs(ctx, EnqueuePendingSemanticJobsParams{
			TargetModel:       targetModel,
			ExcludeCategories: []string{"marketing", "sales", "support", "management"},
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	t.Run("never-embedded open job is enqueued", func(t *testing.T) {
		truncate(t, pool)
		j := insertJob(t, pool, "fresh")
		enqueue(t)
		if got := semanticOutboxJobIDs(t, pool); len(got) != 1 || got[0] != j {
			t.Errorf("enqueued = %v, want [%d]", got, j)
		}
	})

	t.Run("content-changed job is re-enqueued", func(t *testing.T) {
		truncate(t, pool)
		j := insertJob(t, pool, "changed")
		setContentHash(t, pool, j, "new-hash")
		setSemanticStamp(t, pool, j, targetModel, "old-hash") // embedded, but content moved on
		enqueue(t)
		if got := semanticOutboxJobIDs(t, pool); len(got) != 1 || got[0] != j {
			t.Errorf("enqueued = %v, want [%d] (hash mismatch)", got, j)
		}
	})

	t.Run("model-stale job is re-enqueued", func(t *testing.T) {
		truncate(t, pool)
		j := insertJob(t, pool, "modelstale")
		setContentHash(t, pool, j, "h1")
		setSemanticStamp(t, pool, j, "older-model", "h1") // same content, older model
		enqueue(t)
		if got := semanticOutboxJobIDs(t, pool); len(got) != 1 || got[0] != j {
			t.Errorf("enqueued = %v, want [%d] (model mismatch)", got, j)
		}
	})

	t.Run("up-to-date job is not enqueued", func(t *testing.T) {
		truncate(t, pool)
		j := insertJob(t, pool, "current")
		setContentHash(t, pool, j, "h1")
		setSemanticStamp(t, pool, j, targetModel, "h1") // model + hash both match
		enqueue(t)
		if got := semanticOutboxJobIDs(t, pool); len(got) != 0 {
			t.Errorf("enqueued = %v, want none (up to date)", got)
		}
	})

	t.Run("non-tech open job is excluded", func(t *testing.T) {
		truncate(t, pool)
		tech := insertJob(t, pool, "tech")
		setCategory(t, pool, tech, "backend")
		sales := insertJob(t, pool, "sales")
		setCategory(t, pool, sales, "sales")
		empty := insertJob(t, pool, "empty") // keeps '' default → must pass the gate
		enqueue(t)
		got := semanticOutboxJobIDs(t, pool)
		want := []int64{tech, empty}
		sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("enqueued = %v, want %v (sales excluded, empty kept)", got, want)
		}
	})

	t.Run("repeated enqueue does not duplicate", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "idem")
		enqueue(t)
		enqueue(t)
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM semantic_outbox").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("outbox rows = %d, want 1 (one per (job_id, target_model))", n)
		}
	})

	t.Run("closed-but-still-embedded job is enqueued for removal", func(t *testing.T) {
		truncate(t, pool)
		open := insertJob(t, pool, "open")
		goneEmbedded := insertJob(t, pool, "gone-embedded")
		setSemanticStamp(t, pool, goneEmbedded, targetModel, "h1")
		closeJob(t, pool, goneEmbedded)
		goneUnembedded := insertJob(t, pool, "gone-unembedded") // never embedded
		closeJob(t, pool, goneUnembedded)
		enqueue(t)
		got := semanticOutboxJobIDs(t, pool)
		want := []int64{open, goneEmbedded}
		sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("enqueued = %v, want %v (closed+embedded queued for removal, closed+unembedded skipped)", got, want)
		}
	})

	t.Run("closed non-tech embedded job is still enqueued for removal", func(t *testing.T) {
		// The removal branch (Branch B) carries no category gate: a job that closed
		// keeps a live vector until removed regardless of category, so a non-tech job
		// embedded while it was still classified tech must not be stranded in the index.
		truncate(t, pool)
		j := insertJob(t, pool, "gone-nontech")
		setCategory(t, pool, j, "sales")
		setSemanticStamp(t, pool, j, targetModel, "h1")
		closeJob(t, pool, j)
		enqueue(t)
		if got := semanticOutboxJobIDs(t, pool); len(got) != 1 || got[0] != j {
			t.Errorf("enqueued = %v, want [%d] (closed non-tech still removed)", got, j)
		}
	})
}

func TestSemanticClaim(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	enqueue := func(t *testing.T) {
		t.Helper()
		if _, err := q.EnqueuePendingSemanticJobs(ctx, EnqueuePendingSemanticJobsParams{TargetModel: targetModel}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	t.Run("fresher jobs are claimed first", func(t *testing.T) {
		truncate(t, pool)
		older := insertJob(t, pool, "older")
		newer := insertJob(t, pool, "newer")
		setPostedAt(t, pool, older, "2024-01-01T00:00:00Z")
		setPostedAt(t, pool, newer, "2024-06-01T00:00:00Z")
		enqueue(t)
		claimed, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 2 {
			t.Fatalf("claim: rows=%d err=%v, want 2", len(claimed), err)
		}
		if claimed[0].JobID != newer || claimed[1].JobID != older {
			t.Errorf("claim order = [%d, %d], want [%d, %d]", claimed[0].JobID, claimed[1].JobID, newer, older)
		}
	})

	t.Run("undated jobs rank by created_at, not last", func(t *testing.T) {
		// An old dated job vs a freshly-ingested undated one. Under NULLS LAST the
		// undated job would sort last; COALESCE(posted_at, created_at) ranks it by its
		// recent created_at, so it is claimed first — no starvation for undated jobs.
		truncate(t, pool)
		dated := insertJob(t, pool, "dated")
		setPostedAt(t, pool, dated, "2024-01-01T00:00:00Z")
		undated := insertJob(t, pool, "undated") // posted_at NULL, created_at = now()
		enqueue(t)
		claimed, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 2 {
			t.Fatalf("claim: rows=%d err=%v, want 2", len(claimed), err)
		}
		if claimed[0].JobID != undated || claimed[1].JobID != dated {
			t.Errorf("claim order = [%d, %d], want [%d, %d] (undated-but-recent first)",
				claimed[0].JobID, claimed[1].JobID, undated, dated)
		}
	})

	t.Run("closed jobs are claimed and flagged", func(t *testing.T) {
		truncate(t, pool)
		open := insertJob(t, pool, "open")
		gone := insertJob(t, pool, "gone")
		setSemanticStamp(t, pool, gone, targetModel, "h1")
		closeJob(t, pool, gone)
		enqueue(t) // enqueues open (needs embed) + gone (needs removal)
		claimed, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 2 {
			t.Fatalf("claim: rows=%d err=%v, want 2", len(claimed), err)
		}
		byJob := map[int64]bool{}
		for _, c := range claimed {
			byJob[c.JobID] = c.Closed
		}
		if byJob[open] {
			t.Errorf("open job %d flagged closed", open)
		}
		if !byJob[gone] {
			t.Errorf("closed job %d not flagged closed (must be returned for removal)", gone)
		}
	})

	t.Run("claim leases entries so concurrent claims are disjoint", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "j1")
		insertJob(t, pool, "j2")
		enqueue(t)
		first, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 1})
		if err != nil || len(first) != 1 {
			t.Fatalf("first claim: rows=%d err=%v, want 1", len(first), err)
		}
		second, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(second) != 1 {
			t.Fatalf("second claim: rows=%d err=%v, want 1", len(second), err)
		}
		if first[0].ID == second[0].ID {
			t.Errorf("both claims returned outbox id %d — not disjoint", first[0].ID)
		}
	})

	t.Run("a stale lease is reclaimable", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "stale")
		enqueue(t)
		if c, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(c), err)
		}
		if c, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Fatalf("re-claim within lease: rows=%d, want 0", len(c))
		}
		if c, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 1 {
			t.Errorf("re-claim with expired lease: rows=%d err=%v, want 1", len(c), err)
		}
	})
}

func TestSemanticStampClearFailure(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("stamp then clear provenance", func(t *testing.T) {
		truncate(t, pool)
		j := insertJob(t, pool, "stampme")
		setContentHash(t, pool, j, "h1") // the batch stamp copies content_hash
		if err := q.StampSemanticEmbeddedBatch(ctx, StampSemanticEmbeddedBatchParams{Model: targetModel, Ids: []int64{j}}); err != nil {
			t.Fatal(err)
		}
		model, hash := semanticStamp(t, pool, j)
		if model == nil || *model != targetModel || hash == nil || *hash != "h1" {
			t.Errorf("after stamp: model=%v hash=%v, want %q/%q", model, hash, targetModel, "h1")
		}
		if err := q.ClearSemanticEmbeddedBatch(ctx, []int64{j}); err != nil {
			t.Fatal(err)
		}
		if model, hash := semanticStamp(t, pool, j); model != nil || hash != nil {
			t.Errorf("after clear: model=%v hash=%v, want both nil", model, hash)
		}
	})

	t.Run("NULL-content_hash job stamped NULL is not re-enqueued", func(t *testing.T) {
		// A job whose content_hash is NULL (never re-ingested since the content_hash
		// migration) embeds fine; the stamp copies content_hash, so it records NULL — the
		// enqueue's `semantic_embedded_hash IS DISTINCT FROM content_hash` (NULL vs NULL →
		// false) then does not re-queue it forever.
		truncate(t, pool)
		j := insertJob(t, pool, "nullhash") // content_hash stays NULL
		if err := q.StampSemanticEmbeddedBatch(ctx, StampSemanticEmbeddedBatchParams{Model: targetModel, Ids: []int64{j}}); err != nil {
			t.Fatal(err)
		}
		if _, hash := semanticStamp(t, pool, j); hash != nil {
			t.Errorf("semantic_embedded_hash = %q, want NULL", *hash)
		}
		if _, err := q.EnqueuePendingSemanticJobs(ctx, EnqueuePendingSemanticJobsParams{TargetModel: targetModel}); err != nil {
			t.Fatal(err)
		}
		if got := semanticOutboxJobIDs(t, pool); len(got) != 0 {
			t.Errorf("enqueued = %v, want none (NULL hash matches NULL content_hash)", got)
		}
	})

	t.Run("delete entry removes the outbox row", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "del")
		if _, err := q.EnqueuePendingSemanticJobs(ctx, EnqueuePendingSemanticJobsParams{TargetModel: targetModel}); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(claimed), err)
		}
		if err := q.DeleteSemanticEntriesBatch(ctx, []int64{claimed[0].ID}); err != nil {
			t.Fatal(err)
		}
		if got := semanticOutboxJobIDs(t, pool); len(got) != 0 {
			t.Errorf("outbox = %v, want empty after delete", got)
		}
	})

	t.Run("attempts reaching max dead-letters the entry", func(t *testing.T) {
		truncate(t, pool)
		insertJob(t, pool, "dead")
		if _, err := q.EnqueuePendingSemanticJobs(ctx, EnqueuePendingSemanticJobsParams{TargetModel: targetModel}); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: rows=%d err=%v, want 1", len(claimed), err)
		}
		id := claimed[0].ID
		first, err := q.RecordSemanticFailure(ctx, RecordSemanticFailureParams{LastError: "boom", MaxAttempts: 2, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if first.Attempts != 1 || first.FailedAt.Valid {
			t.Errorf("after 1st failure: attempts=%d failed=%v, want 1/not-dead", first.Attempts, first.FailedAt.Valid)
		}
		second, err := q.RecordSemanticFailure(ctx, RecordSemanticFailureParams{LastError: "boom", MaxAttempts: 2, ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if second.Attempts != 2 || !second.FailedAt.Valid {
			t.Errorf("after 2nd failure: attempts=%d failed=%v, want 2/dead-lettered", second.Attempts, second.FailedAt.Valid)
		}
		if c, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 0, BatchSize: 10}); err != nil || len(c) != 0 {
			t.Errorf("claim after dead-letter: rows=%d, want 0", len(c))
		}
	})
}
