//go:build integration

// Integration tests for the incremental-index change signal UpsertJob returns:
// `inserted` (the write created the row) and `changed` (its content_hash differs
// from the stored one). These drive whether ingest re-pushes a job to the live
// search index, and the CTE that captures the pre-update hash is a SQL behavior
// verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ingestUpsert unwraps the embedded Job from UpsertJob's richer row so the many
// existing tests can keep treating the result as a plain Job. The change signal
// (Inserted/Changed) is exercised directly via q.UpsertJob below. (Named to avoid
// the generated `upsertJob` query-string constant.)
func ingestUpsert(ctx context.Context, q *Queries, p UpsertJobParams) (Job, error) {
	row, err := q.UpsertJob(ctx, p)
	return row.Job, err
}

func TestUpsertJobReportsInsertedAndChanged(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	withHash := func(externalID, title, hash string) UpsertJobParams {
		p := ingestParams(externalID, title)
		p.ContentHash = pgtype.Text{String: hash, Valid: true}
		return p
	}

	// First crawl: the row is created.
	r1, err := q.UpsertJob(ctx, withHash("acme:1", "Engineer", "h1"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if !r1.Inserted.Bool {
		t.Error("first write: inserted = false, want true")
	}
	if !r1.Changed {
		t.Error("first write: changed = false, want true (new content)")
	}

	// Re-crawl with identical content (same hash): only bookkeeping moved.
	r2, err := q.UpsertJob(ctx, withHash("acme:1", "Engineer", "h1"))
	if err != nil {
		t.Fatalf("no-op re-upsert: %v", err)
	}
	if r2.Inserted.Bool {
		t.Error("unchanged re-ingest: inserted = true, want false")
	}
	if r2.Changed {
		t.Error("unchanged re-ingest: changed = true, want false")
	}
	if r2.Job.ID != r1.Job.ID {
		t.Fatalf("re-ingest created a new row (%d != %d)", r2.Job.ID, r1.Job.ID)
	}

	// Re-crawl with edited content (new hash): changed, not inserted.
	r3, err := q.UpsertJob(ctx, withHash("acme:1", "Senior Engineer", "h2"))
	if err != nil {
		t.Fatalf("changed re-upsert: %v", err)
	}
	if r3.Inserted.Bool {
		t.Error("edited re-ingest: inserted = true, want false")
	}
	if !r3.Changed {
		t.Error("edited re-ingest: changed = false, want true")
	}
}

// A legacy row whose content_hash is NULL (ingested before the column existed)
// reports changed on its next ingest, so it self-heals into the index.
func TestUpsertJobLegacyNullHashReportsChanged(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Simulate a pre-migration row: insert with no content_hash arg (NULL).
	if _, err := q.UpsertJob(ctx, ingestParams("acme:legacy", "Engineer")); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}

	p := ingestParams("acme:legacy", "Engineer")
	p.ContentHash = pgtype.Text{String: "h1", Valid: true}
	r, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if r.Inserted.Bool {
		t.Error("legacy re-ingest: inserted = true, want false")
	}
	if !r.Changed {
		t.Error("legacy NULL-hash re-ingest: changed = false, want true (self-heal)")
	}
}
