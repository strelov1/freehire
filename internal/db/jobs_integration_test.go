//go:build integration

// Integration tests for the ingest write path's SQL contract: re-ingest must preserve
// enrichment (UpsertJob no longer touches the enrichment columns) and the gated
// transactional enqueue must queue only jobs that still need enriching. These are SQL
// behaviors, verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// jsonEqual compares two JSON documents by value, ignoring Postgres JSONB whitespace
// reformatting.
func jsonEqual(a, b []byte) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func ingestParams(externalID, title string) UpsertJobParams {
	return UpsertJobParams{
		Source:      "greenhouse",
		ExternalID:  externalID,
		URL:         "https://example.test/job",
		Title:       title,
		Company:     "Acme",
		CompanySlug: "acme",
		// Stable per external_id (not per title) so a re-ingest with an edited
		// title carries the same slug — mirroring the pipeline, which mints the
		// slug from (source, external_id), not from volatile fields.
		PublicSlug:  "pslug-" + externalID,
		Location:    "Remote",
		Remote:      true,
		Description: "Build things.",
		PostedAt:    pgtype.Timestamptz{},
	}
}

func TestUpsertJobPreservesEnrichmentOnReingest(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	first, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Old Title"))
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	// The enrichment worker enriches the job out of band.
	enrichment := []byte(`{"seniority":"senior"}`)
	if err := q.SetJobEnrichment(ctx, SetJobEnrichmentParams{
		Enrichment:        enrichment,
		EnrichedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
		EnrichmentVersion: 1,
		ID:                first.ID,
	}); err != nil {
		t.Fatalf("set enrichment: %v", err)
	}

	// Re-ingest the same job (same source+external_id) with an edited title.
	second, err := ingestUpsert(ctx, q, ingestParams("acme:1", "New Title"))
	if err != nil {
		t.Fatalf("re-ingest upsert: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("re-ingest created a new row (id %d != %d) — dedup broken", second.ID, first.ID)
	}
	if second.Title != "New Title" {
		t.Errorf("Title = %q, want the re-ingested value", second.Title)
	}
	if !jsonEqual(second.Enrichment, enrichment) {
		t.Errorf("Enrichment = %s, want preserved %s (re-ingest wiped it)", second.Enrichment, enrichment)
	}
	if second.EnrichmentVersion != 1 {
		t.Errorf("EnrichmentVersion = %d, want preserved 1", second.EnrichmentVersion)
	}
	if !second.EnrichedAt.Valid {
		t.Error("EnrichedAt was cleared by re-ingest, want preserved")
	}
}

func TestEnqueueJobEnrichmentGating(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	outboxCount := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM enrichment_outbox").Scan(&n); err != nil {
			t.Fatalf("count outbox: %v", err)
		}
		return n
	}

	t.Run("unenriched job is enqueued, idempotently", func(t *testing.T) {
		truncate(t, pool)
		job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "A Job"))
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}

		n, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{TargetVersion: 1, JobID: job.ID})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		if n != 1 {
			t.Errorf("first enqueue affected %d rows, want 1", n)
		}

		if _, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{TargetVersion: 1, JobID: job.ID}); err != nil {
			t.Fatalf("second enqueue: %v", err)
		}
		if got := outboxCount(); got != 1 {
			t.Errorf("outbox rows = %d, want 1 (enqueue is idempotent)", got)
		}
	})

	t.Run("already-enriched job is not enqueued", func(t *testing.T) {
		truncate(t, pool)
		job, err := ingestUpsert(ctx, q, ingestParams("acme:2", "Enriched Job"))
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		if err := q.SetJobEnrichment(ctx, SetJobEnrichmentParams{
			Enrichment:        []byte(`{}`),
			EnrichedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
			EnrichmentVersion: 1,
			ID:                job.ID,
		}); err != nil {
			t.Fatalf("set enrichment: %v", err)
		}

		n, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{TargetVersion: 1, JobID: job.ID})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		if n != 0 {
			t.Errorf("enqueue affected %d rows, want 0 (job already at target version)", n)
		}
		if got := outboxCount(); got != 0 {
			t.Errorf("outbox rows = %d, want 0", got)
		}
	})
}

func TestUpsertJobWritesAndRefreshesGeography(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	p := ingestParams("acme:geo", "Geo Job")
	p.Countries = []string{"us"}
	p.Regions = []string{"us"}
	first, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !reflect.DeepEqual(first.Countries, []string{"us"}) || !reflect.DeepEqual(first.Regions, []string{"us"}) {
		t.Fatalf("countries=%v regions=%v, want [us]/[us]", first.Countries, first.Regions)
	}

	// Re-ingest with different geography: EXCLUDED.* refreshes the columns.
	p2 := ingestParams("acme:geo", "Geo Job")
	p2.Countries = []string{"gb"}
	p2.Regions = []string{"uk"}
	second, err := ingestUpsert(ctx, q, p2)
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-ingest created a new row — dedup broken")
	}
	if !reflect.DeepEqual(second.Countries, []string{"gb"}) || !reflect.DeepEqual(second.Regions, []string{"uk"}) {
		t.Errorf("after re-ingest countries=%v regions=%v, want [gb]/[uk]", second.Countries, second.Regions)
	}

	// A posting with no parsed geography (nil args) stores empty arrays via
	// COALESCE, not NULL — and reads back as empty (emit_empty_slices), not nil.
	nilJob, err := ingestUpsert(ctx, q, ingestParams("acme:nilgeo", "No Geo"))
	if err != nil {
		t.Fatalf("upsert nil geo: %v", err)
	}
	if len(nilJob.Countries) != 0 || len(nilJob.Regions) != 0 {
		t.Errorf("countries=%v regions=%v, want empty arrays for unresolved location", nilJob.Countries, nilJob.Regions)
	}
}

func TestUpdateJobFacetsBackfillsAllColumns(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:backfill", "Backfill Job"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(job.Countries) != 0 {
		t.Fatalf("precondition: want empty geography, got %v", job.Countries)
	}

	if err := q.UpdateJobFacets(ctx, UpdateJobFacetsParams{
		ID:        job.ID,
		Countries: []string{"de"},
		Regions:   []string{"eu"},
		WorkMode:  "remote",
		Skills:    []string{"go"},
		Seniority: "senior",
		Category:  "backend",
	}); err != nil {
		t.Fatalf("update facets: %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !reflect.DeepEqual(got.Countries, []string{"de"}) || !reflect.DeepEqual(got.Regions, []string{"eu"}) {
		t.Errorf("after backfill countries=%v regions=%v, want [de]/[eu]", got.Countries, got.Regions)
	}
	if got.WorkMode != "remote" || got.Seniority != "senior" || got.Category != "backend" {
		t.Errorf("scalars = {%q %q %q}, want {remote senior backend}", got.WorkMode, got.Seniority, got.Category)
	}
	if !reflect.DeepEqual(got.Skills, []string{"go"}) {
		t.Errorf("skills = %v, want [go]", got.Skills)
	}
}

func TestListJobsOrdersByNewestAdded(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// "older" is ingested first but carries a NEWER platform posted_at than
	// "newer" — under posted_at ordering it would wrongly stay on top.
	older := ingestParams("order-a", "Older addition")
	older.PostedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if _, err := ingestUpsert(ctx, q, older); err != nil {
		t.Fatal(err)
	}
	newer := ingestParams("order-b", "Newer addition")
	newer.PostedAt = pgtype.Timestamptz{Time: time.Now().Add(-30 * 24 * time.Hour), Valid: true}
	if _, err := ingestUpsert(ctx, q, newer); err != nil {
		t.Fatal(err)
	}

	jobs, err := q.ListJobs(ctx, ListJobsParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	if jobs[0].ExternalID != "order-b" || jobs[1].ExternalID != "order-a" {
		t.Errorf("order = [%s, %s], want newest-added first [order-b, order-a]",
			jobs[0].ExternalID, jobs[1].ExternalID)
	}
}

// TestListJobsOpenOnlyAndEstimate covers the DB-backed /jobs list contract after
// the index+estimate change: ListJobs returns only open jobs (closed_at IS NULL)
// newest-added first, and EstimateOpenJobs returns a non-negative approximate
// open-job total (the Postgres planner estimate, meaningful once stats exist).
func TestListJobsOpenOnlyAndEstimate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Three open jobs, ingested oldest-first so newest-added is est-c.
	for _, id := range []string{"est-a", "est-b", "est-c"} {
		if _, err := ingestUpsert(ctx, q, ingestParams(id, "Title "+id)); err != nil {
			t.Fatal(err)
		}
	}
	// One closed job: it must be excluded from both the list and the estimate.
	closed, err := ingestUpsert(ctx, q, ingestParams("est-closed", "Closed one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closed.ID); err != nil {
		t.Fatal(err)
	}

	jobs, err := q.ListJobs(ctx, ListJobsParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	// len 3 of 4 total (3 open + 1 closed) proves the closed job is excluded.
	if len(jobs) != 3 {
		t.Fatalf("ListJobs len = %d, want 3 (closed excluded)", len(jobs))
	}
	if jobs[0].ExternalID != "est-c" || jobs[2].ExternalID != "est-a" {
		t.Errorf("order = [%s..%s], want newest-added first (est-c..est-a)",
			jobs[0].ExternalID, jobs[2].ExternalID)
	}

	// The planner estimate is only meaningful once the table has statistics.
	if _, err := pool.Exec(ctx, "ANALYZE jobs"); err != nil {
		t.Fatal(err)
	}
	est, err := q.EstimateOpenJobs(ctx)
	if err != nil {
		t.Fatalf("EstimateOpenJobs: %v", err)
	}
	if est < 1 {
		t.Errorf("EstimateOpenJobs = %d, want >= 1 (open jobs present)", est)
	}
}
