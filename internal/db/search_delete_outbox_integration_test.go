//go:build integration

// Integration tests for search_delete_outbox, the queue that removes a closed job's
// document from the facet index.
//
// The load-bearing property here is the one search_outbox does NOT have: an entry must
// outlive the job row it names. cmd/prune hard-deletes jobs by id list with no closed_at
// condition, so mirroring search_outbox's ON DELETE CASCADE would delete a pending removal
// exactly when the job disappeared — stranding that document in the index with nothing left
// in the database that knows it should not be there. Verifiable only against real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// enqueueDeletion queues a removal directly, standing in for the closing statements until
// they carry the CTE themselves.
func enqueueDeletion(t *testing.T, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO search_delete_outbox (job_id) VALUES ($1) ON CONFLICT (job_id) DO NOTHING`, jobID)
	if err != nil {
		t.Fatalf("enqueue deletion for %d: %v", jobID, err)
	}
}

func countDeletionQueue(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM search_delete_outbox`).Scan(&n); err != nil {
		t.Fatalf("count deletion queue: %v", err)
	}
	return n
}

// The regression this whole table's shape exists to prevent.
func TestSearchDeleteOutboxSurvivesItsJobBeingDeleted(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:pruned", "Pruned"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	enqueueDeletion(t, pool, job.ID)

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("hard-delete the job: %v", err)
	}

	if got := countDeletionQueue(t, pool); got != 1 {
		t.Fatalf("deletion queue holds %d rows after the job was deleted, want 1 — "+
			"a foreign key with ON DELETE CASCADE would strand this document in the index", got)
	}
}

func TestClaimSearchDeleteOutboxBatchReturnsQueuedJobIDs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	first, err := ingestUpsert(ctx, q, ingestParams("acme:one", "One"))
	if err != nil {
		t.Fatalf("upsert one: %v", err)
	}
	second, err := ingestUpsert(ctx, q, ingestParams("acme:two", "Two"))
	if err != nil {
		t.Fatalf("upsert two: %v", err)
	}
	enqueueDeletion(t, pool, first.ID)
	enqueueDeletion(t, pool, second.ID)

	claimed, err := q.ClaimSearchDeleteOutboxBatch(ctx, ClaimSearchDeleteOutboxBatchParams{
		LeaseSeconds: 300,
		BatchSize:    10,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d entries, want 2", len(claimed))
	}
}

// A claimed entry is leased, so a second worker running concurrently takes nothing.
func TestClaimSearchDeleteOutboxBatchLeasesWhatItTakes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:leased", "Leased"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	enqueueDeletion(t, pool, job.ID)

	args := ClaimSearchDeleteOutboxBatchParams{LeaseSeconds: 300, BatchSize: 10}
	if _, err := q.ClaimSearchDeleteOutboxBatch(ctx, args); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	again, err := q.ClaimSearchDeleteOutboxBatch(ctx, args)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second claim took %d leased entries, want 0", len(again))
	}
}

func TestCompleteSearchDeleteOutboxRemovesTheEntries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:done", "Done"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	enqueueDeletion(t, pool, job.ID)

	claimed, err := q.ClaimSearchDeleteOutboxBatch(ctx, ClaimSearchDeleteOutboxBatchParams{
		LeaseSeconds: 300,
		BatchSize:    10,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	ids := make([]int64, 0, len(claimed))
	for _, c := range claimed {
		ids = append(ids, c.ID)
	}
	if err := q.CompleteSearchDeleteOutbox(ctx, ids); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if got := countDeletionQueue(t, pool); got != 0 {
		t.Errorf("deletion queue holds %d rows after completion, want 0", got)
	}
}
