//go:build integration

// The semantic queue must treat a non-canonical repost (duplicate_of set) like a closed
// job: never embed it, and remove it if it was embedded while canonical — so it stays
// consistent with the full reindex --semantic, which drops reposts via splitJobs.
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setDuplicateOf(t *testing.T, pool *pgxpool.Pool, jobID, canonID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET duplicate_of = $1 WHERE id = $2", canonID, jobID); err != nil {
		t.Fatalf("set duplicate_of: %v", err)
	}
}

func contains(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestSemanticEnqueue_RepostsExcludedAndRemoved(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	canon := insertJob(t, pool, "canon")
	setContentHash(t, pool, canon, "h1")

	// A never-embedded repost must NOT be enqueued for embedding.
	repost := insertJob(t, pool, "repost")
	setContentHash(t, pool, repost, "h1")
	setDuplicateOf(t, pool, repost, canon)

	// A repost that WAS embedded while canonical must be enqueued for removal.
	embeddedRepost := insertJob(t, pool, "embedded-repost")
	setContentHash(t, pool, embeddedRepost, "h1")
	setSemanticStamp(t, pool, embeddedRepost, targetModel, "h1")
	setDuplicateOf(t, pool, embeddedRepost, canon)

	if _, err := q.EnqueuePendingSemanticJobs(ctx, EnqueuePendingSemanticJobsParams{TargetModel: targetModel}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	ids := semanticOutboxJobIDs(t, pool)
	if !contains(ids, canon) {
		t.Errorf("canon %d not enqueued for embedding", canon)
	}
	if contains(ids, repost) {
		t.Errorf("never-embedded repost %d enqueued, want excluded", repost)
	}
	if !contains(ids, embeddedRepost) {
		t.Errorf("embedded repost %d not enqueued for removal", embeddedRepost)
	}

	// The claim flags the embedded repost for removal (closed=true), like a closed job.
	claimed, err := q.ClaimSemanticBatch(ctx, ClaimSemanticBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, c := range claimed {
		if c.JobID == embeddedRepost && !c.Closed {
			t.Errorf("embedded repost claimed with closed=false, want true (removal)")
		}
		if c.JobID == canon && c.Closed {
			t.Errorf("canon claimed with closed=true, want false (embed)")
		}
	}
}
