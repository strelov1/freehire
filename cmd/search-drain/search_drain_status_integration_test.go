//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
)

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
