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
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meilisearch/meilisearch-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/job/jobview"
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
	meiliURL := "http://" + host + ":" + port.Port()
	if _, err := meilisearch.New(meiliURL, meilisearch.WithAPIKey(key)).ExperimentalFeatures().
		SetEditDocumentsByFunction(true).UpdateWithContext(ctx); err != nil {
		t.Fatalf("enable document editing: %v", err)
	}
	return meiliURL, key
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
// is that closing enqueues a status update and draining it hides the document from
// live search without physically deleting its text and vectors.
func TestIntegration_ClosingAJobHidesItsDocument(t *testing.T) {
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

	deletions := searchdrain.DeletionRunner{Store: newDeletionStore(pool), Deleter: facetDeleter{client: client, q: db.New(pool)}}
	del, err := deletions.Run(ctx, opt)
	if err != nil {
		t.Fatalf("deletion run: %v", err)
	}
	if del.Deleted != 1 || del.Failed != 0 {
		t.Fatalf("deletion stats = %+v, want deleted=1 failed=0", del)
	}

	if found, _ := meiliDoc(t, meiliURL, key, jobID); !found {
		t.Fatalf("job %d was physically deleted instead of marked closed", jobID)
	}
	result, err := client.Search(ctx, search.SearchParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("closed job remains searchable: %+v", result.Hits)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_delete_outbox").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("search_delete_outbox has %d rows, want 0 (drained)", n)
	}
}

// A crawled posting its source has not shown for longer than search.UnseenWindow never
// enters the index, while a fresh posting in the same wave does — the drain asks the same
// search.Withheld the full rebuild does.
func TestIntegration_SearchDrainSkipsAPostingItsSourceStoppedShowing(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	freshID := seedJob(t, pool, "fresh", "Platform Engineer", "acme", "fp-fresh", []string{}, nil)
	staleID := seedJob(t, pool, "stale", "Data Engineer", "acme", "fp-stale", []string{}, nil)
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET source = 'greenhouse',
		        last_seen_at = CASE WHEN id = $2 THEN now() - interval '15 days' ELSE now() END
		  WHERE id = ANY($1)`,
		[]int64{freshID, staleID}, staleID); err != nil {
		t.Fatalf("stamp last_seen_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity a SET last_seen_at=j.last_seen_at
		FROM jobs j WHERE a.job_id=j.id AND j.id=ANY($1)`, []int64{freshID, staleID}); err != nil {
		t.Fatalf("stamp activity last_seen_at: %v", err)
	}
	enqueue(t, pool, freshID)
	enqueue(t, pool, staleID)

	runner := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
	if _, err := runner.Run(ctx, searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if found, _ := meiliDoc(t, meiliURL, key, freshID); !found {
		t.Fatalf("fresh job %d not indexed", freshID)
	}
	if found, _ := meiliDoc(t, meiliURL, key, staleID); found {
		t.Errorf("job %d its source stopped showing 15 days ago reached the index", staleID)
	}
}

func TestIntegration_DeletionPreservesReopenedDocument(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)
	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	jobID := seedJob(t, pool, "reopened", "Platform Engineer", "acme", "fp-reopened", []string{"Lisbon"}, nil)
	enqueue(t, pool, jobID)
	opt := searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}
	indexing := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: q}}
	if _, err := indexing.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CloseJobByID(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at=NULL,closed_reason='' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	removals := searchdrain.DeletionRunner{Store: newDeletionStore(pool), Deleter: facetDeleter{client: client, q: db.New(pool)}}
	if _, err := removals.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if found, _ := meiliDoc(t, meiliURL, key, jobID); !found {
		t.Fatal("stale removal deleted a reopened job")
	}
	if _, err := indexing.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("stale removals=%d, want 0", pending)
	}
}

func TestIntegration_RecloseAfterSkippedRemovalKeepsNewRemoval(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	jobID := seedJob(t, pool, "reclosed", "Engineer", "acme", "fp-reclosed", []string{}, nil)
	if _, err := q.CloseJobByID(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ClaimSearchDeleteOutboxBatch(ctx, db.ClaimSearchDeleteOutboxBatchParams{BatchSize: 10, LeaseSeconds: 180})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at=NULL,closed_reason='' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	// No client is needed: an open canonical job must never reach Meilisearch deletion.
	if err := (facetDeleter{q: q}).DeleteBatch(ctx, []int64{jobID}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CloseJobByID(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if err := q.CompleteSearchDeleteOutbox(ctx, []int64{claimed[0].ID}); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id=$1`, jobID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("new closure lost its removal: pending=%d, want 1", pending)
	}
}

func TestIntegration_HardDeletedJobRemovesItsDocument(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)
	q := db.New(pool)
	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	jobID := seedJob(t, pool, "hard-deleted", "Engineer", "acme", "fp-deleted", []string{}, nil)
	enqueue(t, pool, jobID)
	opt := searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}
	if _, err := (searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: q}}).Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CloseJobByID(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := (searchdrain.DeletionRunner{Store: newDeletionStore(pool), Deleter: facetDeleter{client: client, q: q}}).Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if found, _ := meiliDoc(t, meiliURL, key, jobID); found {
		t.Fatal("hard-deleted posting was retained instead of physically removed")
	}
}

func TestIntegration_DuplicateStatusRetainsContentAndRestoresCanonicalJob(t *testing.T) {
	ctx := context.Background()
	url, key := startMeili(t)
	pool := testdb.Pool(t)
	q := db.New(pool)
	client := search.NewClient(url, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	canon := seedJob(t, pool, "canon", "Canonical Engineer", "acme", "fp-canon", []string{"Berlin"}, nil)
	dup := seedJob(t, pool, "replica", "Replica Engineer", "acme", "fp-replica", []string{"Paris"}, nil)
	absent := seedJob(t, pool, "unindexed-replica", "Replica Engineer", "acme", "fp-absent", []string{}, nil)
	for _, id := range []int64{canon, dup} {
		row, err := q.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		row.Skills = []string{"go", "docker"}
		doc, err := search.FromJob(row, "")
		if err != nil {
			t.Fatal(err)
		}
		doc.Reality = &jobview.Reality{Class: "fresh"}
		if err := client.IndexJobs(ctx, []search.JobDocument{doc}); err != nil {
			t.Fatal(err)
		}
	}
	engine := meilisearch.New(url, meilisearch.WithAPIKey(key)).Index("jobs")
	read := func(id int64) map[string]any {
		t.Helper()
		var doc map[string]any
		if err := engine.GetDocumentWithContext(ctx, strconv.FormatInt(id, 10), &meilisearch.DocumentQuery{RetrieveVectors: true}, &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	before := read(dup)
	for _, id := range []int64{dup, absent} {
		if _, err := q.MarkJobDuplicateOfRole(ctx, db.MarkJobDuplicateOfRoleParams{ID: id, DuplicateOfRole: pgtype.Int8{Int64: canon, Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	opt := searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}
	deleter := facetDeleter{client: client, q: q}
	removals := searchdrain.DeletionRunner{Store: newDeletionStore(pool), Deleter: deleter}
	stats, err := removals.Run(ctx, opt)
	if err != nil || stats.Failed != 0 || stats.Deleted != 2 {
		t.Fatalf("wave=%+v err=%v", stats, err)
	}
	after := read(dup)
	if after["is_duplicate"] != true || after["is_closed"] == true {
		t.Fatalf("status=%v", after)
	}
	if after["title"] != before["title"] || !reflect.DeepEqual(after["_vectors"], before["_vectors"]) {
		t.Fatal("duplicate update lost text or vectors")
	}
	if err := deleter.DeleteBatch(ctx, []int64{dup, absent}); err != nil {
		t.Fatal(err)
	}
	engineStats, err := engine.GetStatsWithContext(ctx, nil)
	if err != nil || engineStats.NumberOfDocuments != 2 {
		t.Fatalf("documents=%+v err=%v; absent duplicate must not create a skeleton", engineStats, err)
	}
	for _, query := range []string{"Engineer", "Replica OR Canonical"} {
		res, err := client.Search(ctx, search.SearchParams{Query: query, Limit: 10})
		if err != nil || res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != canon {
			t.Fatalf("query=%q result=%+v err=%v", query, res, err)
		}
	}
	// A duplicate-only term must contribute neither a result nor any counts.
	facets, err := client.FacetCounts(ctx, search.FacetParams{Query: "Replica", Facets: []string{"company_slug"}})
	if err != nil || facets.Total != 0 {
		t.Fatalf("duplicate facets=%+v err=%v", facets, err)
	}
	count, err := client.DistinctCount(ctx, "Replica", nil, "company_slug")
	if err != nil || count != 0 {
		t.Fatalf("duplicate distinct=%d err=%v", count, err)
	}
	page, total, err := client.ListSitemapPage(ctx, 0, 10)
	if err != nil || total != 1 || len(page) != 1 {
		t.Fatalf("sitemap=%+v total=%d err=%v", page, total, err)
	}
	// Closing a retained duplicate must still apply both independent markers.
	if _, err := q.CloseJobByID(ctx, dup); err != nil {
		t.Fatal(err)
	}
	if _, err := removals.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	both := read(dup)
	if both["is_duplicate"] != true || both["is_closed"] != true {
		t.Fatalf("combined status=%v", both)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at=NULL,closed_reason='' WHERE id=$1`, dup); err != nil {
		t.Fatal(err)
	}
	// Release the duplicate while an old status wave is in flight. Its acknowledgment
	// must repair the late marker with a current-content replacement.
	if _, err := pool.Exec(ctx, `INSERT INTO search_delete_outbox(job_id) VALUES($1)`, dup); err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ClaimSearchDeleteOutboxBatch(ctx, db.ClaimSearchDeleteOutboxBatchParams{BatchSize: 500, LeaseSeconds: 180})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if _, err := q.MarkJobDuplicateOfRole(ctx, db.MarkJobDuplicateOfRoleParams{ID: dup}); err != nil {
		t.Fatal(err)
	}
	runner := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: q}}
	if _, err := runner.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	// Model an older status mutation landing after that canonical replacement.
	if err := client.MarkDuplicateJobs(ctx, []int64{dup}); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseJobs(ctx, []int64{dup}); err != nil {
		t.Fatal(err)
	}
	if err := q.CompleteSearchDeleteOutbox(ctx, []int64{claimed[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	restored, err := client.Search(ctx, search.SearchParams{Query: "Replica", Limit: 10})
	if err != nil || len(restored.Hits) != 1 || restored.Hits[0].ID != dup {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	// Privacy remains a physical-removal boundary, even for a duplicate.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private=true, duplicate_of_role=$2 WHERE id=$1`, dup, canon); err != nil {
		t.Fatal(err)
	}
	if err := deleter.DeleteBatch(ctx, []int64{dup}); err != nil {
		t.Fatal(err)
	}
	if found, _ := meiliDoc(t, url, key, dup); found {
		t.Fatal("private duplicate retained in public index")
	}
}

// A posting that becomes withheld after it was indexed must leave the index on its next
// drain, not at a full rebuild that no longer runs on a schedule.
func TestIntegration_SearchDrainRemovesAJobThatBecameWithheld(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	runner := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: db.New(pool)}}
	opts := searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}

	shownID := seedJob(t, pool, "shown", "Backend Engineer", "acme", "fp-a", []string{"Berlin"}, nil)
	goneID := seedJob(t, pool, "gone", "Backend Engineer", "acme", "fp-b", []string{"Paris"}, nil)
	enqueue(t, pool, shownID)
	enqueue(t, pool, goneID)
	if _, err := runner.Run(ctx, opts); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if found, _ := meiliDoc(t, meiliURL, key, goneID); !found {
		t.Fatalf("job %d not indexed by the first drain", goneID)
	}

	// The body disappears: search.Withheld now says no.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET description = '' WHERE id = $1", goneID); err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, shownID)
	enqueue(t, pool, goneID)
	if stats, err := runner.Run(ctx, opts); err != nil || stats.Failed != 0 {
		t.Fatalf("second Run: stats=%+v err=%v", stats, err)
	}

	if found, _ := meiliDoc(t, meiliURL, key, goneID); found {
		t.Errorf("withheld job %d still indexed after its drain", goneID)
	}
	if found, _ := meiliDoc(t, meiliURL, key, shownID); !found {
		t.Errorf("listable job %d removed along with the withheld one", shownID)
	}
}

// TestIntegration_FacetdDownDuringWaveIsQueuedAndRetried: facetd refuses writes during an
// indexing wave and a deletion wave. Meilisearch still gets both, the job ids land in
// facetd_outbox, and once facetd is back the retry pass sends each job's CURRENT state and
// empties the queue.
func TestIntegration_FacetdDownDuringWaveIsQueuedAndRetried(t *testing.T) {
	ctx := context.Background()
	meiliURL, key := startMeili(t)
	pool := testdb.Pool(t)
	q := db.New(pool)

	client := search.NewClient(meiliURL, key)
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	opts := searchdrain.RunOptions{BatchSize: 500, LeaseSeconds: 180, MaxAttempts: 3}

	openID := seedJob(t, pool, "open", "Backend Engineer", "acme", "fp-o", []string{"Berlin"}, nil)
	closedID := seedJob(t, pool, "closed", "Backend Engineer", "globex", "fp-c", []string{"Paris"}, nil)
	enqueue(t, pool, closedID)
	plain := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: q}}
	if _, err := plain.Run(ctx, opts); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	status := http.StatusServiceUnavailable
	fc, bodies, stop := fakeFacetd(t, &status, 0)
	defer stop()
	mirror := &facetdMirror{client: fc, queue: dbFacetdQueue{q}, timeout: 5 * time.Second}

	enqueue(t, pool, openID)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO search_delete_outbox (job_id) VALUES ($1) ON CONFLICT DO NOTHING`, closedID); err != nil {
		t.Fatal(err)
	}
	runner := searchdrain.Runner{Store: newDBStore(pool), Indexer: searchIndexer{client: client, q: q, mirror: mirror}}
	deletions := searchdrain.DeletionRunner{Store: newDeletionStore(pool), Deleter: facetDeleter{client: client, q: q, mirror: mirror}}
	stats, del, err := searchdrain.Drain(ctx, runner, deletions, opts)
	if err != nil || stats.Failed != 0 || del.Failed != 0 {
		t.Fatalf("Drain: %+v %+v %v", stats, del, err)
	}
	if found, _ := meiliDoc(t, meiliURL, key, openID); !found {
		t.Fatalf("Meilisearch missed job %d though facetd was down", openID)
	}
	queued := func() []int64 {
		rows, err := pool.Query(ctx, `SELECT job_id FROM facetd_outbox ORDER BY job_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	if got := queued(); !slices.Equal(got, []int64{openID, closedID}) {
		t.Fatalf("facetd_outbox = %v, want [%d %d]", got, openID, closedID)
	}

	// facetd is back: the retry pass re-derives current state and empties the queue.
	status = http.StatusOK
	*bodies = nil
	retry := facetdRetry{store: newDBFacetdRetryStore(pool), facetd: fc, build: docBuilder{q}.build, mirror: mirror}
	res := retry.Run(ctx, opts)
	if res.Sent != 2 || res.Failed != 0 || len(*bodies) != 1 {
		t.Fatalf("retry = %+v, writes %d", res, len(*bodies))
	}
	var ups []struct {
		ID int64 `json:"id"`
	}
	b := (*bodies)[0]
	if err := json.Unmarshal(b["upserts"], &ups); err != nil || len(ups) != 1 || ups[0].ID != openID {
		t.Fatalf("upserts = %s (%v), want job %d", b["upserts"], err, openID)
	}
	if string(b["close"]) != fmt.Sprintf("[%d]", closedID) {
		t.Fatalf("close = %s, want [%d]", b["close"], closedID)
	}
	if got := queued(); len(got) != 0 {
		t.Fatalf("facetd_outbox not empty after retry: %v", got)
	}
}
