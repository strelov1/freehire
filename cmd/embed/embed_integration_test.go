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
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
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
	ctx := context.Background()
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)
	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"), postgres.WithUsername("hire"), postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...), postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
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
	t.Setenv("EMBED_URL", fakeTEI(t)) // search.NewClient reads EMBED_URL → stub TEI
	meiliURL, key := startMeili(t)
	pool := startPostgres(t)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	// Seed an open job (to embed) and a closed, already-embedded job (to remove).
	openID := seedJob(t, pool, "open", "Senior Golang Engineer", false, true)
	closedID := seedJob(t, pool, "closed", "Junior Frontend Developer", true, true)
	// Pre-index the closed job's document so removal has something to delete, and stamp
	// it as embedded so the enqueue picks it up for removal.
	preIndexClosed(t, ctx, client, pool, closedID)

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
}

func seedJob(t *testing.T, pool *pgxpool.Pool, ext, title string, closed, withHash bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, description, public_slug,
		                   content_hash, closed_at, enrichment)
		 VALUES ('test', $1, 'http://example.test', $2, 'Acme', 'Build things.', 'job-' || $1,
		         CASE WHEN $3 THEN 'h-' || $1 ELSE NULL END,
		         CASE WHEN $4 THEN now() ELSE NULL END, '{}')
		 RETURNING id`,
		ext, title, withHash, closed).Scan(&id)
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
	if _, err := ix.IndexOpen(ctx, jobs); err != nil {
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
