//go:build integration

// Integration tests for job_semantic_chunks — the pgvector-backed per-chunk embedding
// storage introduced by migration 0092 (see
// openspec/changes/drop-hybrid-search-pgvector-similar/design.md Decisions 1/5). These
// cover the delete/insert "replace" round trip (DeleteJobSemanticChunks +
// InsertJobSemanticChunks) and the similar-jobs nearest-neighbour-over-chunks rollup
// (NearestJobsToJob). This is SQL/pgvector behavior and can only be verified against a
// real Postgres. Run with: go test -tags=integration ./internal/db/ (requires Docker).
// Reuses startPostgres/truncate from enrichment_integration_test.go.
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// insertChunkTestJob inserts a job with an explicit company_slug and returns its id —
// company_slug is the axis these tests exercise (same-company exclusion), which
// insertJob (enrichment_integration_test.go) leaves blank and insertJobForCompany
// (job_collections_integration_test.go) doesn't hand back an id for.
func insertChunkTestJob(t *testing.T, pool *pgxpool.Pool, externalID, companySlug string) int64 {
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

// unitVector768 builds a 768-dim vector literal (job_semantic_chunks.embedding's fixed
// arity) that is 1.0 in exactly one dimension and 0 elsewhere. Two unit vectors along
// the same dimension are identical (cosine distance 0); two along different dimensions
// are orthogonal (cosine distance 1) — this gives full, exact control over the distances
// pgvector's <=> operator reports, without needing real embedding data.
func unitVector768(dim int) string {
	v := make([]float32, 768)
	v[dim] = 1
	return pgvector.NewVector(v).String()
}

// repeatJobID builds the flattened job_ids array InsertJobSemanticChunks now takes: one
// element per chunk (not per job), so a single job's n chunks repeat its id n times.
func repeatJobID(id int64, n int) []int64 {
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = id
	}
	return ids
}

// jobSemanticChunkIndices returns a job's stored chunk_index values, ascending — used to
// assert the delete/insert replace round trip landed the expected rows.
func jobSemanticChunkIndices(t *testing.T, pool *pgxpool.Pool, jobID int64) []int16 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT chunk_index FROM job_semantic_chunks WHERE job_id = $1 ORDER BY chunk_index", jobID)
	if err != nil {
		t.Fatalf("select job_semantic_chunks: %v", err)
	}
	defer rows.Close()
	var got []int16
	for rows.Next() {
		var idx int16
		if err := rows.Scan(&idx); err != nil {
			t.Fatalf("scan chunk_index: %v", err)
		}
		got = append(got, idx)
	}
	return got
}

func TestJobSemanticChunksReplace(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("insert then delete-then-insert replaces the row set", func(t *testing.T) {
		truncate(t, pool)
		j := insertChunkTestJob(t, pool, "replace", "acme")

		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds:       repeatJobID(j, 2),
			ChunkIndices: []int16{0, 1},
			Embeddings:   []string{unitVector768(0), unitVector768(1)},
		}); err != nil {
			t.Fatalf("initial insert: %v", err)
		}
		if got := jobSemanticChunkIndices(t, pool, j); len(got) != 2 || got[0] != 0 || got[1] != 1 {
			t.Fatalf("chunk indices after initial insert = %v, want [0 1]", got)
		}

		// A re-embed produces a different chunk count than before (the source text was
		// re-chunked) — this is exactly why "replace" is delete-then-insert, not an
		// upsert: there is no stable chunk_index target to update onto.
		if err := q.DeleteJobSemanticChunks(ctx, []int64{j}); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds:       repeatJobID(j, 1),
			ChunkIndices: []int16{0},
			Embeddings:   []string{unitVector768(2)},
		}); err != nil {
			t.Fatalf("replace insert: %v", err)
		}

		got := jobSemanticChunkIndices(t, pool, j)
		if len(got) != 1 || got[0] != 0 {
			t.Fatalf("chunk indices after replace = %v, want [0]", got)
		}

		var emb pgvector.Vector
		if err := pool.QueryRow(ctx,
			"SELECT embedding FROM job_semantic_chunks WHERE job_id = $1 AND chunk_index = 0", j).Scan(&emb); err != nil {
			t.Fatalf("read replaced embedding: %v", err)
		}
		if emb.Slice()[2] != 1 {
			t.Errorf("replaced embedding = %v, want dim 2 set to 1 (the new vector, not the old dim-0 one)", emb.Slice()[:3])
		}
	})

	t.Run("delete alone clears a closed job's chunks", func(t *testing.T) {
		truncate(t, pool)
		j := insertChunkTestJob(t, pool, "closepath", "acme")
		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds:       repeatJobID(j, 1),
			ChunkIndices: []int16{0},
			Embeddings:   []string{unitVector768(0)},
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
		if err := q.DeleteJobSemanticChunks(ctx, []int64{j}); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if got := jobSemanticChunkIndices(t, pool, j); len(got) != 0 {
			t.Errorf("chunk indices after delete = %v, want none", got)
		}
	})

	t.Run("batched insert assigns each job its own chunks, no cross-contamination", func(t *testing.T) {
		truncate(t, pool)
		j1 := insertChunkTestJob(t, pool, "flat-1", "acme")
		j2 := insertChunkTestJob(t, pool, "flat-2", "acme")
		// One call across BOTH jobs, flattened: j1 contributes 2 chunks, j2 contributes 1
		// — the shape a real embed wave produces (variable chunk count per job).
		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds:       []int64{j1, j1, j2},
			ChunkIndices: []int16{0, 1, 0},
			Embeddings:   []string{unitVector768(0), unitVector768(1), unitVector768(2)},
		}); err != nil {
			t.Fatalf("batched insert: %v", err)
		}

		if got := jobSemanticChunkIndices(t, pool, j1); len(got) != 2 || got[0] != 0 || got[1] != 1 {
			t.Fatalf("job1 chunk indices = %v, want [0 1]", got)
		}
		if got := jobSemanticChunkIndices(t, pool, j2); len(got) != 1 || got[0] != 0 {
			t.Fatalf("job2 chunk indices = %v, want [0]", got)
		}
		var j2Emb pgvector.Vector
		if err := pool.QueryRow(ctx,
			"SELECT embedding FROM job_semantic_chunks WHERE job_id = $1 AND chunk_index = 0", j2).Scan(&j2Emb); err != nil {
			t.Fatalf("read job2 embedding: %v", err)
		}
		if j2Emb.Slice()[2] != 1 {
			t.Errorf("job2 embedding = %v, want dim 2 set to 1 (its own vector, not job1's)", j2Emb.Slice()[:3])
		}
	})

	t.Run("batched delete clears multiple jobs' chunks in one call", func(t *testing.T) {
		truncate(t, pool)
		j1 := insertChunkTestJob(t, pool, "batch-1", "acme")
		j2 := insertChunkTestJob(t, pool, "batch-2", "acme")
		for _, j := range []int64{j1, j2} {
			if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
				JobIds:       repeatJobID(j, 1),
				ChunkIndices: []int16{0},
				Embeddings:   []string{unitVector768(0)},
			}); err != nil {
				t.Fatalf("insert job %d: %v", j, err)
			}
		}

		if err := q.DeleteJobSemanticChunks(ctx, []int64{j1, j2}); err != nil {
			t.Fatalf("batched delete: %v", err)
		}
		if got := jobSemanticChunkIndices(t, pool, j1); len(got) != 0 {
			t.Errorf("job %d chunk indices after batched delete = %v, want none", j1, got)
		}
		if got := jobSemanticChunkIndices(t, pool, j2); len(got) != 0 {
			t.Errorf("job %d chunk indices after batched delete = %v, want none", j2, got)
		}
	})
}

func TestNearestJobsToJob(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	insertChunks := func(t *testing.T, jobID int64, indices []int16, dims []int) {
		t.Helper()
		embeddings := make([]string, len(dims))
		for i, d := range dims {
			embeddings[i] = unitVector768(d)
		}
		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds: repeatJobID(jobID, len(indices)), ChunkIndices: indices, Embeddings: embeddings,
		}); err != nil {
			t.Fatalf("insert chunks for job %d: %v", jobID, err)
		}
	}

	// Source job: two chunks, unit vectors along dims 0 and 1.
	source := insertChunkTestJob(t, pool, "source", "acme-source")
	insertChunks(t, source, []int16{0, 1}, []int{0, 1})

	// Same company as the source, and its one chunk is an EXACT match (dist 0) for the
	// source's chunk 0 — the closest possible vector match, yet it must be excluded
	// purely because it shares the source's company_slug.
	sameCompany := insertChunkTestJob(t, pool, "same-company", "acme-source")
	insertChunks(t, sameCompany, []int16{0}, []int{0})

	// Different company. Its FIRST chunk (dim 5) is far from both of the source's
	// chunks (orthogonal, dist 1); its SECOND chunk exactly matches the source's chunk
	// 1 (dim 1, dist 0). Proves the MIN-across-chunks rollup is real: the winning pair
	// is (source.chunk1, candidate.chunk_index=1), not chunk_index 0 vs 0.
	trueNearest := insertChunkTestJob(t, pool, "true-nearest", "beta-co")
	insertChunks(t, trueNearest, []int16{0, 1}, []int{5, 1})

	// Different company, closed. Its chunk is an exact match (dist 0) for the source's
	// chunk 0 — would rank first by vector distance alone, but must be excluded because
	// it is closed.
	closedMatch := insertChunkTestJob(t, pool, "closed-match", "gamma-co")
	insertChunks(t, closedMatch, []int16{0}, []int{0})
	closeJob(t, pool, closedMatch)

	// Different company, open, orthogonal to both source chunks (dist 1 from either) —
	// a background candidate that should rank behind trueNearest (dist 0) but still
	// appear in the result (open, different company).
	farAway := insertChunkTestJob(t, pool, "far-away", "delta-co")
	insertChunks(t, farAway, []int16{0}, []int{6})

	rows, err := q.NearestJobsToJob(ctx, NearestJobsToJobParams{JobID: source, LimitCount: 10})
	if err != nil {
		t.Fatalf("NearestJobsToJob: %v", err)
	}

	got := map[int64]float64{}
	var order []int64
	for _, r := range rows {
		got[r.JobID] = r.Distance
		order = append(order, r.JobID)
	}

	if _, ok := got[sameCompany]; ok {
		t.Errorf("same-company job %d present in results %v, want excluded even though it is the closest vector match", sameCompany, order)
	}
	if _, ok := got[closedMatch]; ok {
		t.Errorf("closed job %d present in results %v, want excluded", closedMatch, order)
	}
	if len(order) != 2 || order[0] != trueNearest || order[1] != farAway {
		t.Fatalf("result order = %v, want [%d(true-nearest) %d(far-away)]", order, trueNearest, farAway)
	}
	if d := got[trueNearest]; d > 1e-6 {
		t.Errorf("trueNearest distance = %v, want ~0 (its chunk_index=1 exactly matches source chunk_index=1)", d)
	}
	if d := got[farAway]; d < 0.999 || d > 1.001 {
		t.Errorf("farAway distance = %v, want ~1 (orthogonal)", d)
	}
}
