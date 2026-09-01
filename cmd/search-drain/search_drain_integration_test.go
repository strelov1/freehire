//go:build integration

// End-to-end test for the incremental facet-search worker: real Postgres
// (testcontainers, migrations applied) + real Meilisearch (testcontainers). It drives
// the real dbStore + searchIndexer + searchdrain.Runner over seeded jobs and asserts
// the queued job's document lands in the `jobs` index, widened with its role
// cluster's geography — the same guarantee the old inline ingest push carried,
// moved here with the push itself. Run with:
//
//	go test -tags=integration ./cmd/search-drain/   (requires Docker)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
)

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

// seedJob inserts a minimal job row directly (bypassing cmd/ingest), so the worker's
// own dbStore + searchIndexer are what get exercised, not the write path.
func seedJob(t *testing.T, pool *pgxpool.Pool, ext, title, companySlug, roleFingerprint string, cities []string, duplicateOf *int64) int64 {
	t.Helper()
	var id int64
	// duplicate_of_role, not duplicate_of: migration 0115 derives duplicate_of from the three
	// OWNED marker columns on every insert and update, so a fixture writing the derived column
	// directly has it silently overwritten with NULL and seeds a canonical row instead of the
	// repost it meant to. That is not hypothetical — this fixture did exactly that until the
	// geography union started reading duplicate_of, and nothing failed.
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, description,
		                   public_slug, content_hash, role_fingerprint, cities, duplicate_of_role, enrichment, category)
		 VALUES ('test', $1, 'http://example.test', $2, 'Acme', $3, 'Build things.',
		         'job-' || $1, 'h-' || $1, $4, $5, $6, '{}', 'backend')
		 RETURNING id`,
		ext, title, companySlug, roleFingerprint, cities, duplicateOf).Scan(&id)
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

func enqueue(t *testing.T, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	if err := db.New(pool).EnqueueSearchOutbox(context.Background(), jobID); err != nil {
		t.Fatalf("enqueue job %d: %v", jobID, err)
	}
}

// meiliDoc fetches a document from the `jobs` facet index, decoding only the fields
// this test asserts on.
func meiliDoc(t *testing.T, meiliURL, key string, id int64) (found bool, cities []string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/indexes/jobs/documents/%d", meiliURL, id), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get document %d: unexpected status %d", id, resp.StatusCode)
	}
	var body struct {
		Cities []string `json:"cities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode document %d: %v", id, err)
	}
	return true, body.Cities
}

func TestIntegration_SearchDrainWorkerWidensCanonWithClusterGeography(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// The same role posted once per city: the canon and a repost POINTING AT IT. What makes
	// them one group is the marker, not the shared fingerprint — the closure walks
	// duplicate_of, so this would hold just as well if the repost's fingerprint differed,
	// which is the case a fingerprint-keyed union could never see (issue #2225).
	canonID := seedJob(t, pool, "canon", "Senior Backend Engineer", "acme", "fp-1", []string{"Berlin"}, nil)
	repostID := seedJob(t, pool, "repost", "Senior Backend Engineer", "acme", "fp-1", []string{"Paris"}, &canonID)

	// Only the canon is queued — a non-canonical repost never reaches search, mirroring
	// the gate cmd/ingest applies before enqueuing.
	enqueue(t, pool, canonID)

	runner := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
	stats, err := runner.Run(ctx, searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want indexed=1 failed=0", stats)
	}

	found, cities := meiliDoc(t, meiliURL, key, canonID)
	if !found {
		t.Fatalf("canon job %d not found in the jobs index after drain", canonID)
	}
	want := []string{"Berlin", "Paris"}
	slices.Sort(cities)
	if !slices.Equal(cities, want) {
		t.Errorf("canon document cities = %v, want the cluster union %v — the drain must widen "+
			"the canon with its repost's geography, not push its own narrow set", cities, want)
	}

	if found, _ := meiliDoc(t, meiliURL, key, repostID); found {
		t.Errorf("repost job %d must never reach the index directly", repostID)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_outbox").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("search_outbox has %d rows, want 0 (drained)", n)
	}
}

// TestIntegration_SearchDrainWorkerBatchesRoleClusterLookupsWithoutCrossContamination
// covers IndexBatch's wave-scoped lookups: two different companies sharing the exact same
// role_fingerprint string, plus a canon representing nobody, all draining in the SAME wave.
//
// The geography half of this can no longer go wrong the way it once could — the closure is
// keyed by job id, and an id belongs to one company by construction, so cross-contamination
// through a shared fingerprint is not expressible. What still keys by
// (company_slug, role_fingerprint) is RoleClusterCountsFor, feeding the job-reality signal,
// and that is what this now guards: drop company_slug from that key and acme's counts pick up
// globex's rows. The geography assertions stay as the regression net for the id keying.
func TestIntegration_SearchDrainWorkerBatchesRoleClusterLookupsWithoutCrossContamination(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// acme/fp-shared: canon + repost, same shape as the test above.
	acmeCanonID := seedJob(t, pool, "acme-canon", "Senior Backend Engineer", "acme", "fp-shared", []string{"Berlin"}, nil)
	seedJob(t, pool, "acme-repost", "Senior Backend Engineer", "acme", "fp-shared", []string{"Paris"}, &acmeCanonID)

	// globex/fp-shared: a DIFFERENT company using the identical role_fingerprint string,
	// with no repost of its own — its geography must stay exactly what it was seeded
	// with, not pick up acme's cities (or vice versa).
	globexCanonID := seedJob(t, pool, "globex-canon", "Senior Backend Engineer", "globex", "fp-shared", []string{"Tokyo"}, nil)

	// A true singleton (no repost, no fingerprint collision) in the same wave, to prove
	// the batched geo query is correctly skipped/no-op for it too.
	soloID := seedJob(t, pool, "solo", "Staff Platform Engineer", "acme", "fp-solo", []string{"Warsaw"}, nil)

	enqueue(t, pool, acmeCanonID)
	enqueue(t, pool, globexCanonID)
	enqueue(t, pool, soloID)

	runner := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
	stats, err := runner.Run(ctx, searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 3 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want indexed=3 failed=0", stats)
	}

	found, cities := meiliDoc(t, meiliURL, key, acmeCanonID)
	if !found {
		t.Fatalf("acme canon job %d not found in the jobs index after drain", acmeCanonID)
	}
	slices.Sort(cities)
	if want := []string{"Berlin", "Paris"}; !slices.Equal(cities, want) {
		t.Errorf("acme canon cities = %v, want %v — must not pick up globex's city despite the "+
			"shared role_fingerprint", cities, want)
	}

	found, cities = meiliDoc(t, meiliURL, key, globexCanonID)
	if !found {
		t.Fatalf("globex canon job %d not found in the jobs index after drain", globexCanonID)
	}
	if want := []string{"Tokyo"}; !slices.Equal(cities, want) {
		t.Errorf("globex canon cities = %v, want %v (its own, unwidened) — must not pick up "+
			"acme's cities despite the shared role_fingerprint", cities, want)
	}

	found, cities = meiliDoc(t, meiliURL, key, soloID)
	if !found {
		t.Fatalf("solo job %d not found in the jobs index after drain", soloID)
	}
	if want := []string{"Warsaw"}; !slices.Equal(cities, want) {
		t.Errorf("solo job cities = %v, want %v (untouched singleton)", cities, want)
	}
}

// The end-to-end guarantee this change exists for: a job that closes stops being searchable
// without waiting for a rebuild. Everything before the close is setup — what is being proven
// is that closing enqueues a removal and that draining it takes the document out of the live
// index.
func TestIntegration_ClosingAJobRemovesItsDocument(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobID := seedJob(t, pool, "closing", "Platform Engineer", "acme", "fp-close", []string{"Lisbon"}, nil)
	enqueue(t, pool, jobID)

	opt := searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}
	indexer := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
	if _, err := indexer.Run(ctx, opt); err != nil {
		t.Fatalf("index run: %v", err)
	}
	if found, _ := meiliDoc(t, meiliURL, key, jobID); !found {
		t.Fatalf("job %d is not in the index before the close, so this test proves nothing", jobID)
	}

	// The close is what queues the removal — no separate call, it rides the statement.
	if _, err := db.New(pool).CloseJobByID(ctx, jobID); err != nil {
		t.Fatalf("close: %v", err)
	}

	deletions := searchdrain.DeletionRunner{Store: newDeletionStore(pool), Deleter: facetDeleter{client: client}}
	del, err := deletions.Run(ctx, opt)
	if err != nil {
		t.Fatalf("deletion run: %v", err)
	}
	if del.Deleted != 1 || del.Failed != 0 {
		t.Fatalf("deletion stats = %+v, want deleted=1 failed=0", del)
	}

	if found, _ := meiliDoc(t, meiliURL, key, jobID); found {
		t.Errorf("job %d is still in the index after being closed and drained", jobID)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_delete_outbox").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("search_delete_outbox has %d rows, want 0 (drained)", n)
	}
}
