//go:build integration

// Integration tests for the Meilisearch-backed search package: EnsureIndex
// (settings + embedder), IndexJobs, and Search (keyword, faceted, hybrid). These
// exercise behavior that only a real engine exhibits. Run with:
//
//	go test -tags=integration ./internal/search/
//
// Requires Docker (testcontainers spins up a throwaway Meilisearch). The first
// run is slow: the huggingFace embedder downloads its model at index time.
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

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
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
		d, err := FromJob(j)
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

	t.Run("hybrid search hits the separate semantic index", func(t *testing.T) {
		// Semantic search rides a second index built by the --semantic pass (the
		// facet index above carries no embedder), so build and populate it here.
		if err := c.EnsureSemanticIndex(ctx); err != nil {
			t.Fatalf("EnsureSemanticIndex: %v", err)
		}
		if err := c.IndexSemanticJobs(ctx, docs); err != nil {
			t.Fatalf("IndexSemanticJobs: %v", err)
		}
		res, err := c.Search(ctx, SearchParams{Query: "backend engineering role", SemanticRatio: 0.5, Limit: 10})
		if err != nil {
			t.Fatalf("hybrid Search: %v", err)
		}
		if len(res.Hits) == 0 {
			t.Error("hybrid search returned no hits")
		}
	})
}

// TestIntegration_EnsureIndexResetsExistingEmbedder guards the merge-semantics
// trap: facetSettings omits the embedder, but a Meilisearch settings update only
// MERGES, so an embedder a prior version put on the `jobs` index would survive and
// keep embedding on every facet reindex. EnsureIndex must reset it explicitly.
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
	if emb, err := c.facet.GetEmbeddersWithContext(ctx); err != nil || len(emb) == 0 {
		t.Fatalf("precondition: embedder should be set (emb=%v err=%v)", emb, err)
	}

	// EnsureIndex must strip it, leaving the facet index embedder-free.
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex (reset): %v", err)
	}
	emb, err := c.facet.GetEmbeddersWithContext(ctx)
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}
	if len(emb) != 0 {
		t.Errorf("EnsureIndex left embedders on the facet index: %v", emb)
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
		d, err := FromJob(j)
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

// SimilarJobs returns the nearest neighbours of a job from the semantic index:
// other jobs are returned (ranked by embedding proximity), the source job is never
// in its own list, and the caller's limit is honoured.
func TestIntegration_SimilarJobs(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureSemanticIndex(ctx); err != nil {
		t.Fatalf("EnsureSemanticIndex: %v", err)
	}

	jobs := []db.Job{
		{ID: 1, Title: "Senior Golang Backend Engineer", Company: "Acme", Location: "Berlin",
			Description: "Build distributed backend services and APIs in Go.",
			PublicSlug:  "senior-golang-backend-engineer-acme-aaa",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 2, Title: "Backend Software Engineer (Go)", Company: "Beta", Location: "Remote",
			Description: "Design server-side microservices and REST APIs in Golang.",
			PublicSlug:  "backend-software-engineer-go-beta-bbb",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 3, Title: "Frontend React Developer", Company: "Gamma", Location: "Remote",
			Description: "Build user interfaces with React and TypeScript.",
			PublicSlug:  "frontend-react-developer-gamma-ccc",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
		{ID: 4, Title: "Data Scientist", Company: "Delta", Location: "London",
			Description: "Train machine learning models and analyse datasets in Python.",
			PublicSlug:  "data-scientist-delta-ddd",
			Enrichment:  enrichedJSON(t, enrich.Enrichment{})},
	}
	if err := c.IndexSemanticJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexSemanticJobs: %v", err)
	}

	t.Run("returns neighbours and excludes the source job", func(t *testing.T) {
		hits, err := c.SimilarJobs(ctx, 1, 10)
		if err != nil {
			t.Fatalf("SimilarJobs: %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("SimilarJobs returned no neighbours")
		}
		for _, h := range hits {
			if h.ID == 1 {
				t.Errorf("source job id 1 must not appear in its own similar list: %+v", hits)
			}
		}
	})

	t.Run("honours the limit", func(t *testing.T) {
		hits, err := c.SimilarJobs(ctx, 1, 2)
		if err != nil {
			t.Fatalf("SimilarJobs: %v", err)
		}
		if len(hits) > 2 {
			t.Errorf("limit 2 returned %d hits", len(hits))
		}
	})

	// The semantic index is built incrementally, so a job can exist in Postgres
	// (and thus reach this call) without a vector in the index yet. Meilisearch
	// answers that with a 400 not_found_similar_id, which must degrade to an empty
	// list — not a hard error — so the detail page just hides the section.
	t.Run("source absent from the index yields empty, not an error", func(t *testing.T) {
		hits, err := c.SimilarJobs(ctx, 9999, 10) // never indexed
		if err != nil {
			t.Fatalf("SimilarJobs for an unindexed id must not error: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no neighbours for an unindexed id, got %d", len(hits))
		}
	})
}

func toDocs(t *testing.T, jobs []db.Job) []JobDocument {
	t.Helper()
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j)
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
