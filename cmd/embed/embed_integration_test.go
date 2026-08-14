//go:build integration

// End-to-end test for the incremental semantic-embedding worker: real Postgres
// (testcontainers, migrations applied) + real Meilisearch (testcontainers) + a stub TEI
// (bag-of-words) the search client is pointed at via EMBED_URL. It drives the real
// dbStore + searchIndexer + embed.Runner over seeded open and closed jobs and asserts
// the open job's vector lands in jobs_semantic, the closed job's document is removed,
// provenance is stamped/cleared, and the outbox drains. Run with:
//
//	go test -tags=integration ./cmd/embed/   (requires Docker)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/strelov1/freehire/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/search"
)

// embedderDimensions is the e5-base vector width Meili validates userProvided vectors
// against (kept in sync with internal/search's unexported const of the same value).
const embedderDimensions = 768

// fakeTEI serves a deterministic bag-of-words vector per input so the stub stays
// model-free while keeping the width Meili accepts. Mirrors internal/search's test stub.
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
	v := make([]float64, embedderDimensions)
	for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		v[h.Sum32()%embedderDimensions]++
	}
	return v
}

func startMeili(t *testing.T) (url, key string) {
	t.Helper()
	ctx := context.Background()
	key = "test-master-key"
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "getmeili/meilisearch:v1.13",
			ExposedPorts: []string{"7700/tcp"},
			Env:          map[string]string{"MEILI_MASTER_KEY": key, "MEILI_ENV": "development"},
			WaitingFor:   wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start meilisearch: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := container.MappedPort(ctx, "7700")
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	return "http://" + host + ":" + port.Port(), key
}

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// meiliDocExists reports whether the semantic index holds a document with the given id.
func meiliDocExists(t *testing.T, meiliURL, key string, id int64) bool {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/indexes/jobs_semantic/documents/%d", meiliURL, id), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true
	case http.StatusNotFound:
		return false
	default:
		t.Fatalf("get document %d: unexpected status %d", id, resp.StatusCode)
		return false
	}
}

func TestIntegration_EmbedWorkerDrainsQueue(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := startPostgres(t)

	client := search.NewClient(meiliURL, key, search.WithEmbedURL(fakeTEI(t))) // stub TEI
	if err := client.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	// Seed an open job (to embed) and a closed, already-embedded job (to remove).
	openID := seedJob(t, pool, "open", "Senior Golang Engineer", false, true)
	closedID := seedJob(t, pool, "closed", "Junior Frontend Developer", true, true)
	// Pre-index the closed job's document so removal has something to delete, and stamp
	// it as embedded so the enqueue picks it up for removal.
	preIndexClosed(t, ctx, client, pool, closedID)

	// Simulate a stale precomputed similar-jobs list on the open job (as if a prior
	// cmd/similar-backfill pass had run) — the embed worker must null it out alongside
	// the rest of its stamp, since a re-embed's chunk set makes the old list stale.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET similar_computed_at = now() WHERE id = $1", openID); err != nil {
		t.Fatalf("seed similar_computed_at: %v", err)
	}

	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
	stats, err := runner.Run(ctx, embed.RunOptions{
		TargetModel: search.CurrentEmbedderModel(), BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 1 || stats.Removed != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want indexed=1 removed=1 failed=0", stats)
	}

	if !meiliDocExists(t, meiliURL, key, openID) {
		t.Errorf("open job %d not in jobs_semantic after embed", openID)
	}
	if meiliDocExists(t, meiliURL, key, closedID) {
		t.Errorf("closed job %d still in jobs_semantic after removal", closedID)
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

	// Durability: the open job's vector is persisted to Postgres beside the stamp (the
	// backup copy that lets the index be rehydrated without re-embedding); the removed
	// job carries none.
	if l := jobVectorLen(t, pool, openID); l <= 0 {
		t.Errorf("open job semantic_embedding length = %d, want > 0 (vector persisted)", l)
	}
	if l := jobVectorLen(t, pool, closedID); l != 0 {
		t.Errorf("closed job semantic_embedding length = %d, want 0/NULL (cleared)", l)
	}

	// Outbox drained.
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM semantic_outbox").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("semantic_outbox has %d rows, want 0 (drained)", n)
	}

	// The additive pgvector chunk pipeline: the open job's short description ("Build
	// things.") yields exactly one chunk row; the closed job (never given chunks by
	// preIndexClosed, which only computes vectors, never persists) has none.
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
	meiliURL, key := startMeili(t)
	pool := startPostgres(t)

	client := search.NewClient(meiliURL, key, search.WithEmbedURL(fakeTEI(t)))
	if err := client.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	longDescription := strings.Repeat(
		"<p>We build large-scale distributed systems in Go, Postgres, and Kubernetes. "+
			"You will own services end to end, from design through on-call.</p>", 60)
	id := seedJobWithDescription(t, pool, "long", "Staff Backend Engineer", longDescription, false, true)

	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
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
	meiliURL, key := startMeili(t)
	pool := startPostgres(t)

	client := search.NewClient(meiliURL, key, search.WithEmbedURL(fakeTEI(t)))
	if err := client.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	id := seedJob(t, pool, "reembed", "Senior Golang Engineer", false, true)
	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
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
}

// TestIntegration_EmbedWorkerDeletesChunksOnClose proves the closed-job path deletes a
// job's job_semantic_chunks rows in the same transaction as the existing
// ClearSemanticEmbeddedBatch stamp-clear.
func TestIntegration_EmbedWorkerDeletesChunksOnClose(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := startPostgres(t)

	client := search.NewClient(meiliURL, key, search.WithEmbedURL(fakeTEI(t)))
	if err := client.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	id := seedJob(t, pool, "toclose", "Senior Golang Engineer", false, true)
	runner := embed.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
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

// preIndexClosed embeds+indexes the closed job's document (so there is a document to
// remove) and stamps it embedded (so the enqueue queues it for removal). It uses the
// same indexer path, then closes the job in PG afterwards via the seed's closed flag.
func preIndexClosed(t *testing.T, ctx context.Context, client *search.Client, pool *pgxpool.Pool, id int64) {
	t.Helper()
	// The job was seeded already-closed; temporarily treat it as open for the pre-index
	// by loading and indexing its document directly.
	ix := searchIndexer{client: client, q: db.New(pool)}
	jobs, err := newDBStore(pool).Jobs(ctx, []int64{id})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("load closed job: rows=%d err=%v", len(jobs), err)
	}
	if _, _, err := ix.IndexOpen(ctx, jobs); err != nil {
		t.Fatalf("pre-index closed job: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET semantic_embedded_model = $1, semantic_embedded_hash = content_hash WHERE id = $2",
		search.CurrentEmbedderModel(), id); err != nil {
		t.Fatalf("stamp closed job: %v", err)
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

// PG-only mode embeds to Postgres WITHOUT touching Meilisearch: the open job's vector
// and stamp land in Postgres, but its document never appears in jobs_semantic (that
// index is filled later by `reindex --semantic --from-pg`). This is the fast bulk-embed
// path that Meili's serial task queue cannot gate.
func TestIntegration_EmbedWorkerPGOnly(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := startPostgres(t)

	client := search.NewClient(meiliURL, key, search.WithEmbedURL(fakeTEI(t)))
	// Create the (empty) semantic index up front so we can assert pg-only leaves it empty.
	if err := client.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	openID := seedJob(t, pool, "pgonly-open", "Senior Golang Engineer", false, true)

	runner := embed.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client, q: db.New(pool), pgOnly: true},
	}
	stats, err := runner.Run(ctx, embed.RunOptions{
		TargetModel: search.CurrentEmbedderModel(), BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 1 {
		t.Fatalf("stats = %+v, want indexed=1", stats)
	}

	// Postgres has the vector + the current-model stamp.
	if l := jobVectorLen(t, pool, openID); l <= 0 {
		t.Errorf("open job semantic_embedding length = %d, want > 0 (vector persisted)", l)
	}
	if model, _ := jobStamp(t, pool, openID); model == nil || *model != search.CurrentEmbedderModel() {
		t.Errorf("open job stamp model = %v, want %q", model, search.CurrentEmbedderModel())
	}
	// Meili was NOT touched: the document is absent from jobs_semantic.
	if meiliDocExists(t, meiliURL, key, openID) {
		t.Error("pg-only mode must NOT write the document into jobs_semantic")
	}
}
