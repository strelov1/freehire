//go:build integration

// End-to-end test for the incremental semantic-embedding worker: real Postgres
// (testcontainers, migrations applied) + a stub TEI (bag-of-words) the search client
// is pointed at via EMBED_URL. It drives the real dbStore + searchIndexer +
// embed.Runner over seeded open and closed jobs and asserts the open job's
// pgvector-backed chunk rows are persisted, the closed job's embed state
// is cleared, provenance is stamped/cleared, and the outbox drains. No Meilisearch is
// involved — cmd/embed no longer writes any search index (see
// openspec/changes/drop-hybrid-search-pgvector-similar). Run with:
//
//	go test -tags=integration ./cmd/embed/   (requires Docker)
package main

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/strelov1/freehire/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/search"
)

// vectorWidth is the vector width the stub embedder returns — matches the real e5-base
// model's width, though nothing here validates it against an index (there is none).
const vectorWidth = 768

// fakeTEI serves a deterministic bag-of-words vector per input so the stub stays
// model-free.
func fakeTEI(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Inputs []string `json:"inputs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := struct {
			Embeddings [][]float64 `json:"embeddings"`
		}{}
		for _, s := range in.Inputs {
			out.Embeddings = append(out.Embeddings, bagOfWords(s))
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func bagOfWords(s string) []float64 {
	v := make([]float64, vectorWidth)
	for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		v[h.Sum32()%vectorWidth]++
	}
	return v
}

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// newTestClient builds a search.Client pointed at the stub TEI, with no Meilisearch
// wiring — cmd/embed's Indexer never touches Meilisearch, so url/key are irrelevant.
func newTestClient(t *testing.T) *search.Client {
	t.Helper()
	return search.NewClient("", "", search.WithEmbedURL(fakeTEI(t)))
}

func TestIntegration_EmbedWorkerDrainsQueue(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	client := newTestClient(t)

	// Seed an open job (to embed) and a closed job already stamped as previously
	// embedded (to have its embed state cleared).
	openID := seedJob(t, pool, "open", "Senior Golang Engineer", false, true)
	closedID := seedJob(t, pool, "closed", "Junior Frontend Developer", true, true)
	seedPreviouslyEmbedded(t, ctx, pool, closedID)

	// Simulate a stale precomputed similar-jobs list on the open job (as if a prior
	// cmd/similar-backfill pass had run) — the embed worker must null it out alongside
	// the rest of its stamp, since a re-embed's chunk set makes the old list stale.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET similar_computed_at = now() WHERE id = $1", openID); err != nil {
		t.Fatalf("seed similar_computed_at: %v", err)
	}

	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client}}
	stats, err := runner.Run(ctx, embed.RunOptions{
		TargetModel: search.CurrentEmbedderModel(), BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 1 || stats.Removed != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want indexed=1 removed=1 failed=0", stats)
	}

	// Provenance: open stamped with the current model AND the exact embedded content_hash
	// (the seed set content_hash = 'h-open'), closed cleared.
	if model, hash := jobStamp(t, pool, openID); model == nil || *model != search.CurrentEmbedderModel() {
		t.Errorf("open job stamp model = %v, want %q", model, search.CurrentEmbedderModel())
	} else if hash == nil || *hash != "h-open" {
		t.Errorf("open job stamp hash = %v, want %q (the embedded content_hash)", hash, "h-open")
	}
	if model, _ := jobStamp(t, pool, closedID); model != nil {
		t.Errorf("closed job stamp model = %v, want NULL (cleared)", model)
	}

	// The OLD jobs.semantic_embedding column is no longer WRITTEN by this pipeline at
	// all (job_semantic_chunks is the sole representation now) — the open job's column
	// stays untouched/NULL, it was never populated to begin with. The closed job was
	// seeded (seedPreviouslyEmbedded) with a legacy vector from a simulated prior run
	// under the old pipeline, specifically to prove ClearSemanticEmbeddedBatch's
	// close-path clear still nulls out any such pre-existing value even though nothing
	// writes new ones anymore — that clear is still real, needed behavior. The column
	// itself stays in the schema (dropping it is a separate, later change).
	if l := jobVectorLen(t, pool, openID); l != 0 {
		t.Errorf("open job semantic_embedding length = %d, want 0/NULL (no longer written)", l)
	}
	if l := jobVectorLen(t, pool, closedID); l != 0 {
		t.Errorf("closed job semantic_embedding length = %d, want 0/NULL (cleared from its seeded legacy value)", l)
	}

	// Outbox drained.
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM semantic_outbox").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("semantic_outbox has %d rows, want 0 (drained)", n)
	}

	// The pgvector chunk pipeline: the open job's short description ("Build things.")
	// yields exactly one chunk row; the closed job (never given chunks by
	// seedPreviouslyEmbedded, which only stamps provenance and a legacy vector) has none.
	if got := jobChunkCount(t, pool, openID); got != 1 {
		t.Errorf("open job chunk rows = %d, want 1 (short description → one chunk)", got)
	}
	if got := jobChunkCount(t, pool, closedID); got != 0 {
		t.Errorf("closed job chunk rows = %d, want 0", got)
	}
	// The precomputed similar-jobs staleness stamp is cleared by the re-embed.
	if !jobSimilarComputedAtIsNull(t, pool, openID) {
		t.Errorf("open job similar_computed_at not cleared by embed")
	}
}

// jobChunkCount returns how many job_semantic_chunks rows a job has.
func jobChunkCount(t *testing.T, pool *pgxpool.Pool, id int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM job_semantic_chunks WHERE job_id = $1", id).Scan(&n); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	return n
}

// jobChunkIndices returns a job's chunk_index values in ascending order.
func jobChunkIndices(t *testing.T, pool *pgxpool.Pool, id int64) []int16 {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT chunk_index FROM job_semantic_chunks WHERE job_id = $1 ORDER BY chunk_index", id)
	if err != nil {
		t.Fatalf("query chunk indices: %v", err)
	}
	defer rows.Close()
	var out []int16
	for rows.Next() {
		var idx int16
		if err := rows.Scan(&idx); err != nil {
			t.Fatalf("scan chunk index: %v", err)
		}
		out = append(out, idx)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate chunk indices: %v", err)
	}
	return out
}

// jobSimilarComputedAtIsNull reports whether a job's similar_computed_at column is NULL.
func jobSimilarComputedAtIsNull(t *testing.T, pool *pgxpool.Pool, id int64) bool {
	t.Helper()
	var ts *time.Time
	if err := pool.QueryRow(context.Background(),
		"SELECT similar_computed_at FROM jobs WHERE id = $1", id).Scan(&ts); err != nil {
		t.Fatalf("read similar_computed_at: %v", err)
	}
	return ts == nil
}

// TestIntegration_EmbedWorkerChunksLongDescriptionIntoMultipleRows proves the whole,
// HTML-stripped description reaches the embedder as several chunk rows, not one
// truncated vector — the quality fix design.md folds into this change (proposal.md
// motivation #2: only ~15-20% of an average description used to reach the model).
func TestIntegration_EmbedWorkerChunksLongDescriptionIntoMultipleRows(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	client := newTestClient(t)

	longDescription := strings.Repeat(
		"<p>We build large-scale distributed systems in Go, Postgres, and Kubernetes. "+
			"You will own services end to end, from design through on-call.</p>", 60)
	id := seedJobWithDescription(t, pool, "long", "Staff Backend Engineer", longDescription, false, true)

	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client}}
	if _, err := runner.Run(ctx, embed.RunOptions{
		TargetModel: search.CurrentEmbedderModel(), BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := jobChunkCount(t, pool, id); got < 2 {
		t.Fatalf("long-description job chunk rows = %d, want > 1", got)
	}
	indices := jobChunkIndices(t, pool, id)
	for i, idx := range indices {
		if int(idx) != i {
			t.Fatalf("chunk indices = %v, want a clean 0..%d sequence", indices, len(indices)-1)
		}
	}
}

// TestIntegration_EmbedWorkerReplacesChunksOnReembedNotAppend proves a re-embed (content
// changed, so semantic_embedded_hash goes stale) REPLACES a job's chunk rows rather than
// appending to them — a job must never end up with a mix of old and new chunk vectors.
func TestIntegration_EmbedWorkerReplacesChunksOnReembedNotAppend(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	client := newTestClient(t)

	id := seedJob(t, pool, "reembed", "Senior Golang Engineer", false, true)
	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client}}
	opts := embed.RunOptions{TargetModel: search.CurrentEmbedderModel(), BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3}
	if _, err := runner.Run(ctx, opts); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	firstCount := jobChunkCount(t, pool, id)
	if firstCount == 0 {
		t.Fatalf("first embed produced no chunk rows")
	}

	// Content change: a much longer description AND a new content_hash, so the
	// staleness predicate (semantic_embedded_hash IS DISTINCT FROM content_hash)
	// re-enqueues the job.
	longDescription := strings.Repeat("Completely different content about distributed systems. ", 100)
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET description = $1, content_hash = 'h-open-v2' WHERE id = $2", longDescription, id); err != nil {
		t.Fatalf("update job content: %v", err)
	}
	if _, err := runner.Run(ctx, opts); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	secondCount := jobChunkCount(t, pool, id)
	if secondCount <= firstCount {
		t.Fatalf("re-embed with much longer content produced %d chunks, want more than the first embed's %d",
			secondCount, firstCount)
	}
	// A clean 0..n-1 chunk_index sequence is proof of replace-not-append: an append bug
	// would leave duplicate/overlapping indices (DeleteJobSemanticChunks+InsertJobSemanticChunks
	// use job_id+chunk_index as the primary key, so a raw append would violate it — but a
	// bug that skipped the delete and reused fresh indices from 0 would silently duplicate
	// rows without violating the PK, which the exact-sequence check below also catches).
	indices := jobChunkIndices(t, pool, id)
	for i, idx := range indices {
		if int(idx) != i {
			t.Fatalf("chunk indices after re-embed = %v, want a clean 0..%d sequence (replace, not append)",
				indices, len(indices)-1)
		}
	}

	// The reverse direction: a re-embed that SHRINKS the chunk set must not leave
	// trailing rows from the larger prior set behind — a bug that replaced only
	// indices 0..newCount-1 (instead of deleting the whole prior set first) would pass
	// the growing-set assertions above but leave stale high-index rows here.
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET description = $1, content_hash = 'h-open-v3' WHERE id = $2", "Short again.", id); err != nil {
		t.Fatalf("update job content (shrink): %v", err)
	}
	if _, err := runner.Run(ctx, opts); err != nil {
		t.Fatalf("third Run: %v", err)
	}
	thirdIndices := jobChunkIndices(t, pool, id)
	if len(thirdIndices) != 1 || thirdIndices[0] != 0 {
		t.Fatalf("chunk indices after shrinking re-embed = %v, want exactly [0] (stale trailing rows not removed)",
			thirdIndices)
	}
}

// TestIntegration_EmbedWorkerDeletesChunksOnClose proves the closed-job path deletes a
// job's job_semantic_chunks rows in the same transaction as the existing
// ClearSemanticEmbeddedBatch stamp-clear.
func TestIntegration_EmbedWorkerDeletesChunksOnClose(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	client := newTestClient(t)

	id := seedJob(t, pool, "toclose", "Senior Golang Engineer", false, true)
	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client}}
	opts := embed.RunOptions{TargetModel: search.CurrentEmbedderModel(), BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3}
	if _, err := runner.Run(ctx, opts); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if got := jobChunkCount(t, pool, id); got == 0 {
		t.Fatalf("open embed produced no chunk rows")
	}

	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", id); err != nil {
		t.Fatalf("close job: %v", err)
	}
	if _, err := runner.Run(ctx, opts); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if got := jobChunkCount(t, pool, id); got != 0 {
		t.Fatalf("chunk rows after close = %d, want 0 (deleted)", got)
	}
}

func seedJob(t *testing.T, pool *pgxpool.Pool, ext, title string, closed, withHash bool) int64 {
	t.Helper()
	return seedJobWithDescription(t, pool, ext, title, "Build things.", closed, withHash)
}

func seedJobWithDescription(t *testing.T, pool *pgxpool.Pool, ext, title, description string, closed, withHash bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, description, public_slug,
		                   content_hash, closed_at, enrichment, is_tech)
		 VALUES ('test', $1, 'http://example.test', $2, 'Acme', $5, 'job-' || $1,
		         CASE WHEN $3 THEN 'h-' || $1 ELSE NULL END,
		         CASE WHEN $4 THEN now() ELSE NULL END, '{}', true)
		 RETURNING id`,
		ext, title, withHash, closed, description).Scan(&id)
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// seedPreviouslyEmbedded stamps a job's provenance (model + content_hash) and gives it
// a legacy vector directly via SQL, as if a prior embed run had already processed it —
// so the enqueue's staleness/removal predicate picks it up as "previously embedded, now
// closed" without needing to drive it through a real IndexOpen call first.
func seedPreviouslyEmbedded(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) {
	t.Helper()
	vec := make([]float32, vectorWidth)
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET semantic_embedded_model = $1, semantic_embedded_hash = content_hash,
		                 semantic_embedding = $2
		 WHERE id = $3`,
		search.CurrentEmbedderModel(), vec, id); err != nil {
		t.Fatalf("seed previously-embedded job: %v", err)
	}
}

// jobVectorLen returns the length of a job's persisted semantic_embedding, or 0 when
// the column is NULL (no vector).
func jobVectorLen(t *testing.T, pool *pgxpool.Pool, id int64) int {
	t.Helper()
	var n *int
	if err := pool.QueryRow(context.Background(),
		"SELECT array_length(semantic_embedding, 1) FROM jobs WHERE id = $1", id).Scan(&n); err != nil {
		t.Fatalf("read vector length: %v", err)
	}
	if n == nil {
		return 0
	}
	return *n
}

func jobStamp(t *testing.T, pool *pgxpool.Pool, id int64) (model, hash *string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		"SELECT semantic_embedded_model, semantic_embedded_hash FROM jobs WHERE id = $1", id).
		Scan(&model, &hash); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	return model, hash
}
