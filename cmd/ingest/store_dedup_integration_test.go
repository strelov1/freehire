//go:build integration

// Integration test for the write-path role dedup: a crawl that lands the same vacancy
// once per city must leave ONE searchable posting, not one per city.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/testdb"
)

// cityPosting builds one city's copy of a single vacancy: the title and description —
// the two fields RoleFingerprint hashes — are identical across copies, only the board
// identity and the location differ. This is the real shape of the AgileEngine posting
// that fanned 30 copies into one subscription digest.
func cityPosting(externalID, location string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      "zohorecruit",
			ExternalID:  externalID,
			Title:       "Senior Full Stack Engineer ID78855",
			Company:     "AgileEngine",
			Location:    location,
			Description: "<div>We are looking for a Senior Full Stack Engineer with a backend orientation.</div>",
		},
		URL: "https://agileengine.zohorecruit.com/jobs/Careers/" + externalID,
	})
	if err != nil {
		panic(err)
	}
	return j
}

// enrichmentQueued reports whether the enrichment outbox holds a row for this job.
func enrichmentQueued(t *testing.T, pool *pgxpool.Pool, jobID int64) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM enrichment_outbox WHERE job_id = $1`, jobID).Scan(&n); err != nil {
		t.Fatalf("count enrichment_outbox for job %d: %v", jobID, err)
	}
	return n > 0
}

func TestSave_CollapsesPerCityCopiesOfOneRole(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)

	if err := store.Save(ctx, cityPosting("248544000257794970", "Querétaro, Mexico")); err != nil {
		t.Fatalf("save the first city: %v", err)
	}
	if err := store.Save(ctx, cityPosting("248544000257794973", "Bogota, Colombia")); err != nil {
		t.Fatalf("save the second city: %v", err)
	}

	first, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
		Source: "zohorecruit", ExternalID: "248544000257794970",
	})
	if err != nil {
		t.Fatalf("load the first city: %v", err)
	}
	second, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
		Source: "zohorecruit", ExternalID: "248544000257794973",
	})
	if err != nil {
		t.Fatalf("load the second city: %v", err)
	}

	// Both copies are kept as rows — the city is what a seeker picks from — but only the
	// older one is canonical.
	if first.DuplicateOf.Valid {
		t.Errorf("the first city is the canon, got duplicate_of = %d", first.DuplicateOf.Int64)
	}
	if !second.DuplicateOf.Valid || second.DuplicateOf.Int64 != first.ID {
		t.Errorf("second.duplicate_of = %v, want the first city's id %d", second.DuplicateOf, first.ID)
	}

	// The consequence that reached the user: a non-canonical copy must never be queued
	// for the live index, or it becomes its own "new job" in every subscription digest.
	if got := searchOutboxCount(t, pool, first.ID); got != 1 {
		t.Errorf("search_outbox entries for the canon = %d, want 1", got)
	}
	if got := searchOutboxCount(t, pool, second.ID); got != 0 {
		t.Errorf("search_outbox entries for the non-canonical copy = %d, want 0", got)
	}

	// And it must not be enriched: an invisible row would pay for an LLM call.
	if !enrichmentQueued(t, pool, first.ID) {
		t.Error("the canon must be queued for enrichment")
	}
	if enrichmentQueued(t, pool, second.ID) {
		t.Error("a non-canonical copy must not be queued for enrichment")
	}
}
