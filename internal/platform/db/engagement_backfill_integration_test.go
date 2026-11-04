//go:build integration

// Integration tests for the cmd/backfill-engagement write. The pass is re-run after every
// dictionary change and relies on two SQL behaviours only a real Postgres can confirm: the
// IS DISTINCT FROM guard, and the search_outbox entry it leaves behind.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func TestSetJobEngagement(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Java Developer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM search_outbox"); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}

	res, err := q.SetJobEngagement(ctx, SetJobEngagementParams{JobID: job.ID, Engagement: []string{"b2b"}})
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if res.UpdatedCount != 1 || res.EnqueuedCount != 1 {
		t.Fatalf("first write = %+v, want one row written and queued", res)
	}

	// The guard: the same terms again write and queue nothing.
	res, err = q.SetJobEngagement(ctx, SetJobEngagementParams{JobID: job.ID, Engagement: []string{"b2b"}})
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if res.UpdatedCount != 0 || res.EnqueuedCount != 0 {
		t.Fatalf("second write = %+v, want nothing — the guard did not hold", res)
	}

	// An entry already pending is re-stamped, not left alone: a reindex purge or a drain
	// holding the old generation must not swallow the new terms.
	if _, err := pool.Exec(ctx,
		"UPDATE search_outbox SET created_at = now() - interval '1 day' WHERE job_id = $1", job.ID); err != nil {
		t.Fatalf("age the entry: %v", err)
	}
	var oldID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM search_outbox WHERE job_id = $1", job.ID).Scan(&oldID); err != nil {
		t.Fatalf("read entry: %v", err)
	}
	res, err = q.SetJobEngagement(ctx, SetJobEngagementParams{JobID: job.ID, Engagement: []string{"b2b", "uop"}})
	if err != nil {
		t.Fatalf("third write: %v", err)
	}
	if res.UpdatedCount != 1 || res.EnqueuedCount != 1 {
		t.Fatalf("third write = %+v, want one row written and its entry re-stamped", res)
	}
	var newID int64
	var fresh bool
	if err := pool.QueryRow(ctx,
		"SELECT id, created_at > now() - interval '1 minute' FROM search_outbox WHERE job_id = $1", job.ID).Scan(&newID, &fresh); err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if newID == oldID || !fresh {
		t.Fatalf("entry id %d -> %d, fresh=%v; want a new generation", oldID, newID, fresh)
	}

	stored, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := stored.Engagement; len(got) != 2 || got[0] != "b2b" || got[1] != "uop" {
		t.Fatalf("engagement = %v, want [b2b uop]", got)
	}
}
