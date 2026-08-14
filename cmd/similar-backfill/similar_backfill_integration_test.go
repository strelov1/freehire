//go:build integration

// End-to-end test for the similar-jobs precompute worker: real Postgres
// (testcontainers, migrations applied via internal/testdb) driving the actual
// dbStore + similarjobs.Runner over seeded jobs with job_semantic_chunks rows. This
// exercises the already-built, already-unit-tested NearestJobsToJob query (see
// internal/db/semantic_chunks_integration_test.go) THROUGH the new worker, so it
// mostly proves the wiring: PendingJobIDs finds the right jobs, NearestJobs/
// SetSimilarJobIDs round-trip correctly, and a second run is a no-op. Run with:
//
//	go test -tags=integration ./cmd/similar-backfill/   (requires Docker)
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/similarjobs"
	"github.com/strelov1/freehire/internal/testdb"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// unitVector768 builds a 768-dim vector literal that is 1.0 in exactly one dimension
// and 0 elsewhere, mirroring internal/db's own test helper of the same name (not
// reusable across packages) — gives exact, deterministic control over pgvector's <=>
// distances without needing real embedding data.
func unitVector768(dim int) string {
	v := make([]float32, 768)
	v[dim] = 1
	return pgvector.NewVector(v).String()
}

func insertTestJob(t *testing.T, pool *pgxpool.Pool, externalID, companySlug string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2) RETURNING id`,
		externalID, companySlug).Scan(&id)
	if err != nil {
		t.Fatalf("insert job %q: %v", externalID, err)
	}
	return id
}

func insertTestChunks(t *testing.T, q *db.Queries, jobID int64, dims []int) {
	t.Helper()
	jobIDs := make([]int64, len(dims))
	indices := make([]int16, len(dims))
	embeddings := make([]string, len(dims))
	for i, d := range dims {
		jobIDs[i] = jobID
		indices[i] = int16(i)
		embeddings[i] = unitVector768(d)
	}
	if err := q.InsertJobSemanticChunks(context.Background(), db.InsertJobSemanticChunksParams{
		JobIds: jobIDs, ChunkIndices: indices, Embeddings: embeddings,
	}); err != nil {
		t.Fatalf("insert chunks for job %d: %v", jobID, err)
	}
}

func readSimilar(t *testing.T, pool *pgxpool.Pool, jobID int64) (ids []int64, computed bool) {
	t.Helper()
	var idsCol []int64
	var computedAt *string
	if err := pool.QueryRow(context.Background(),
		"SELECT similar_job_ids, similar_computed_at::text FROM jobs WHERE id = $1", jobID).
		Scan(&idsCol, &computedAt); err != nil {
		t.Fatalf("read similar columns: %v", err)
	}
	return idsCol, computedAt != nil
}

func runOpts() similarjobs.RunOptions {
	return similarjobs.RunOptions{BatchSize: 100, NeighbourCount: 10, Concurrency: 2}
}

// TestIntegration_SimilarBackfillExcludesSameCompany proves the worker writes the
// company-diverse neighbour list end to end: a same-company job that would otherwise
// be the closest possible vector match (distance 0) never appears, while a
// different-company job with a real (non-zero) distance does.
func TestIntegration_SimilarBackfillExcludesSameCompany(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := db.New(pool)

	source := insertTestJob(t, pool, "source", "acme")
	insertTestChunks(t, q, source, []int{0})

	// Same company, exact vector match — must be excluded purely on company_slug.
	sameCompany := insertTestJob(t, pool, "same-company", "acme")
	insertTestChunks(t, q, sameCompany, []int{0})

	// Different company, a real (non-exact) match — must appear.
	otherCompany := insertTestJob(t, pool, "other-company", "beta")
	insertTestChunks(t, q, otherCompany, []int{1})

	runner := similarjobs.Runner{Store: newDBStore(pool)}
	stats, err := runner.Run(ctx, runOpts())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Processed != 3 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want processed=3 failed=0 (source, same-company, and other-company all have chunks)", stats)
	}

	ids, computed := readSimilar(t, pool, source)
	if !computed {
		t.Fatalf("source job's similar_computed_at not stamped")
	}
	if len(ids) != 1 || ids[0] != otherCompany {
		t.Fatalf("source job similar_job_ids = %v, want [%d] (other-company only, same-company excluded)", ids, otherCompany)
	}
}

// TestIntegration_SimilarBackfillEmptyWhenOnlySameCompanyMatches proves a job whose
// only close matches are same-company yields a short/empty list, not an error, and is
// still stamped as computed.
func TestIntegration_SimilarBackfillEmptyWhenOnlySameCompanyMatches(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := db.New(pool)

	source := insertTestJob(t, pool, "lonely-source", "acme")
	insertTestChunks(t, q, source, []int{5})

	// The only other job with chunks shares the source's company — no eligible
	// candidate exists at all.
	sameCompany := insertTestJob(t, pool, "lonely-same-company", "acme")
	insertTestChunks(t, q, sameCompany, []int{5})

	runner := similarjobs.Runner{Store: newDBStore(pool)}
	stats, err := runner.Run(ctx, runOpts())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 0 {
		t.Fatalf("stats = %+v, want failed=0 (an empty neighbour list is a valid outcome, not an error)", stats)
	}

	ids, computed := readSimilar(t, pool, source)
	if !computed {
		t.Fatalf("source job's similar_computed_at not stamped despite an empty result")
	}
	if len(ids) != 0 {
		t.Fatalf("source job similar_job_ids = %v, want empty (only candidate is same-company)", ids)
	}
}

// TestIntegration_SimilarBackfillRerunIsNoOp proves re-running the worker after a
// full pass does no further work — the incremental predicate (similar_computed_at IS
// NULL) already excludes every job the first pass just stamped (design.md Decision
// 4's incremental design).
func TestIntegration_SimilarBackfillRerunIsNoOp(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := db.New(pool)

	source := insertTestJob(t, pool, "rerun-source", "acme")
	insertTestChunks(t, q, source, []int{0})
	other := insertTestJob(t, pool, "rerun-other", "beta")
	insertTestChunks(t, q, other, []int{1})

	runner := similarjobs.Runner{Store: newDBStore(pool)}
	first, err := runner.Run(ctx, runOpts())
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.Processed == 0 {
		t.Fatalf("first run processed=0, want > 0")
	}

	second, err := runner.Run(ctx, runOpts())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second.Processed != 0 || second.Failed != 0 {
		t.Fatalf("second run stats = %+v, want all zero (nothing pending after a full pass)", second)
	}
}
