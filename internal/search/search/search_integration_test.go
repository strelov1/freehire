//go:build integration

// Integration tests for the Meilisearch-backed search package: EnsureIndex
// (settings), IndexJobs, and Search (keyword, faceted). These exercise behavior
// that only a real engine exhibits. Run with:
//
//	go test -tags=integration ./internal/search/
//
// Requires Docker (testcontainers spins up a throwaway Meilisearch).
package search

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

func startMeili(t *testing.T) *Client {
	t.Helper()
	ctx := context.Background()
	const key = "test-master-key"

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
	return NewClient("http://"+host+":"+port.Port(), key)
}

func enrichedJSON(t *testing.T, e enrich.Enrichment) []byte {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal enrichment: %v", err)
	}
	return raw
}

func TestIntegration_EnsureIndexIndexAndSearch(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 1, Title: "Senior Golang Engineer", Company: "Acme", Location: "Berlin",
			Remote: true, Description: "Build backend services in Go.",
			PublicSlug: "senior-golang-engineer-acme-aaa",
			// seniority/category are dictionary columns (served dict-only).
			Seniority:  "senior",
			Category:   "backend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 2, Title: "Junior Frontend Developer", Company: "Beta", Location: "Remote",
			Remote: true, Description: "React and TypeScript UI work.",
			PublicSlug: "junior-frontend-developer-beta-bbb",
			Seniority:  "junior",
			Category:   "frontend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, skillvec.Weights{})
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	t.Run("keyword matches and strips nothing from the document", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{Query: "golang", Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "senior-golang-engineer-acme-aaa" {
			t.Fatalf("keyword search hits = %+v", res.Hits)
		}
		if res.Hits[0].ID != 1 {
			t.Errorf("hit ID = %d, want 1 (kept internally as PK)", res.Hits[0].ID)
		}
	})

	t.Run("facet filter narrows by nested seniority", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{
			Filter: Filter([]string{Eq("enrichment.seniority", "senior")}),
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Hits) != 1 || res.Hits[0].Enrichment.Seniority != "senior" {
			t.Fatalf("filtered hits = %+v", res.Hits)
		}
	})

	t.Run("sort by posted_at string orders chronologically", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{Sort: []string{"posted_at:desc"}, Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Hits) != 2 || res.Hits[0].PublicSlug != "junior-frontend-developer-beta-bbb" {
			t.Fatalf("posted_at:desc order = %+v", res.Hits)
		}
	})

	t.Run("reindex is idempotent", func(t *testing.T) {
		if err := c.IndexJobs(ctx, docs); err != nil {
			t.Fatalf("re-IndexJobs: %v", err)
		}
		res, err := c.Search(ctx, SearchParams{Limit: 100})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 2 {
			t.Errorf("Total after re-index = %d, want 2", res.Total)
		}
	})

	t.Run("deleting a closed job removes it from the index", func(t *testing.T) {
		if err := c.DeleteJobs(ctx, []int64{2}); err != nil {
			t.Fatalf("DeleteJobs: %v", err)
		}
		res, err := c.Search(ctx, SearchParams{Limit: 100})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 1 || res.Hits[0].ID != 1 {
			t.Fatalf("after delete: total=%d hits=%+v, want only job 1", res.Total, res.Hits)
		}
		// Idempotent: deleting an id that is no longer indexed is a no-op.
		if err := c.DeleteJobs(ctx, []int64{2}); err != nil {
			t.Fatalf("re-DeleteJobs: %v", err)
		}
		// Reopened job: indexing it again restores the document.
		if err := c.IndexJobs(ctx, docs[1:2]); err != nil {
			t.Fatalf("re-IndexJobs reopened: %v", err)
		}
		res, err = c.Search(ctx, SearchParams{Limit: 100})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 2 {
			t.Errorf("after reopen: total=%d, want 2", res.Total)
		}
	})
}

// TestIntegration_EnsureIndexResetsExistingEmbedder guards the merge-semantics trap:
// a Meilisearch settings update only MERGES, and merges embedders BY KEY, so one a
// prior version put on the `jobs` index would survive every update that does not name
// it — and keep embedding on every facet reindex. EnsureIndex must strip it.
//
// It must strip it WITHOUT taking the skill embedder with it, which is the whole
// reason the reset runs before the settings rather than after (see ensure()).
func TestIntegration_EnsureIndexResetsExistingEmbedder(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// Put an embedder on the facet index. userProvided needs no model download.
	task, err := c.facet.UpdateEmbeddersWithContext(ctx, map[string]meilisearch.Embedder{
		"manual": {Source: "userProvided", Dimensions: 3},
	})
	if err != nil {
		t.Fatalf("UpdateEmbedders: %v", err)
	}
	if _, err := c.facet.WaitForTaskWithContext(ctx, task.TaskUID, 50*time.Millisecond); err != nil {
		t.Fatalf("await embedder set: %v", err)
	}
	emb, err := c.facet.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("precondition: GetEmbedders: %v", err)
	}
	if _, ok := emb["manual"]; !ok {
		t.Fatalf("precondition: the inherited embedder should be set, got %v", emb)
	}

	// EnsureIndex must strip the inherited one and leave the skill embedder standing —
	// not leave the index embedder-free, which is what it used to do.
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex (reset): %v", err)
	}
	emb, err = c.facet.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}
	if _, inherited := emb["manual"]; inherited {
		t.Errorf("EnsureIndex left the inherited embedder on the facet index: %v", emb)
	}
	if _, ours := emb[SkillEmbedder]; !ours {
		t.Errorf("EnsureIndex stripped the skill embedder along with the inherited one: %v", emb)
	}
}

func TestSearchFiltersBySkillsFacet(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 10, Title: "Go Engineer", Company: "Acme", Location: "Berlin",
			PublicSlug: "go-engineer-acme-aaa",
			Skills:     []string{"go", "kubernetes"},
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 11, Title: "Python Developer", Company: "Beta", Location: "Remote",
			PublicSlug: "python-developer-beta-bbb",
			Skills:     []string{"python"},
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, skillvec.Weights{})
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{
		Filter: Filter([]string{Eq("skills", "go")}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with skills filter: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "go-engineer-acme-aaa" {
		t.Fatalf("skills facet filter hits = %+v, want only go-engineer-acme-aaa", res.Hits)
	}
}

// The "posted within N days" freshness filter works end-to-end: posted_ts is
// indexed as a numeric, filterable attribute, so a Meilisearch range filter built
// from posted_within_days returns only the recent posting and drops the stale one.
func TestSearchFiltersByPostedWithinDays(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	now := time.Now()
	jobs := []db.Job{
		{
			ID: 20, Title: "Fresh Role", Company: "Acme", PublicSlug: "fresh-role-acme",
			PostedAt:   pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 21, Title: "Stale Role", Company: "Beta", PublicSlug: "stale-role-beta",
			PostedAt:   pgtype.Timestamptz{Time: now.Add(-60 * 24 * time.Hour), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, skillvec.Weights{})
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{
		Filter: FilterFromValues(vals("posted_within_days=7")),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with posted_within_days filter: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "fresh-role-acme" {
		t.Fatalf("freshness filter hits = %+v, want only fresh-role-acme", res.Hits)
	}
}

// A job carries its company's curated collections as a top-level facet: filtering
// on collections returns only the tagged jobs, and a facet distribution over
// collections reports their counts.
func TestSearchFiltersByCollectionsFacet(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 20, Title: "Founding Engineer", Company: "Stripe", Location: "Remote",
			PublicSlug:  "founding-engineer-stripe-aaa",
			Collections: []string{"yc", "bigtech"},
			PostedAt:    pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment:  enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 21, Title: "Backend Engineer", Company: "Acme", Location: "Remote",
			PublicSlug: "backend-engineer-acme-bbb",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}

	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, skillvec.Weights{})
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{
		Filter: Filter([]string{Eq("collections", "yc")}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with collections filter: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "founding-engineer-stripe-aaa" {
		t.Fatalf("collections facet filter hits = %+v, want only founding-engineer-stripe-aaa", res.Hits)
	}

	fres, err := c.FacetCounts(ctx, FacetParams{Facets: []string{"collections"}})
	if err != nil {
		t.Fatalf("FacetCounts: %v", err)
	}
	if fres.Facets["collections"]["yc"] != 1 || fres.Facets["collections"]["bigtech"] != 1 {
		t.Errorf("collections dist = %v, want yc:1 bigtech:1", fres.Facets["collections"])
	}
}

func toDocs(t *testing.T, jobs []db.Job) []JobDocument {
	t.Helper()
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, skillvec.Weights{})
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	return docs
}

// A full rebuild builds a brand-new index and swaps it over the live one in one
// atomic step: documents only in the OLD set must vanish (a wholesale replace, not
// a merge), the fresh set must be searchable, and the throwaway rebuild index must
// be dropped afterwards.
func TestIntegration_RebuildSwapsFreshIndexAndDropsOld(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	// Seed the live index with an OLD set: ids 1 and 2.
	old := toDocs(t, []db.Job{
		{ID: 1, Title: "Old One", PublicSlug: "old-one-aaa", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
		{ID: 2, Title: "Old Two", PublicSlug: "old-two-bbb", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
	})
	if err := c.IndexJobs(ctx, old); err != nil {
		t.Fatalf("IndexJobs (seed): %v", err)
	}

	// Rebuild with a DIFFERENT set: ids 2 and 3 (id 1 dropped, id 3 new).
	r := c.NewFacetRebuild()
	if err := r.Prepare(ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	fresh := toDocs(t, []db.Job{
		{ID: 2, Title: "Two", PublicSlug: "two-bbb", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
		{ID: 3, Title: "Three", PublicSlug: "three-ccc", Enrichment: enrichedJSON(t, enrich.Enrichment{})},
	})
	if err := r.Push(ctx, fresh); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := r.Promote(ctx); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	res, err := c.Search(ctx, SearchParams{Limit: 100})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := map[int64]bool{}
	for _, h := range res.Hits {
		got[h.ID] = true
	}
	if got[1] {
		t.Error("id 1 still present; the swap should have dropped the old-only doc")
	}
	if !got[2] || !got[3] {
		t.Errorf("fresh set missing: got ids %v, want 2 and 3", got)
	}
	if len(res.Hits) != 2 {
		t.Errorf("hit count = %d, want 2 (the fresh set only)", len(res.Hits))
	}

	if _, err := c.manager.GetIndexWithContext(ctx, "jobs_rebuild"); err == nil {
		t.Error("jobs_rebuild still exists; Promote should drop it")
	}
}
