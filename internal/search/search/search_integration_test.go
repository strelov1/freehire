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
	"net/url"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
)

func startMeili(t *testing.T) *Client {
	t.Helper()
	ctx := context.Background()
	const key = "test-master-key"

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			// The version prod runs (docker-compose.yml). Federated OR search depends on
			// engine behaviour, so the test engine must not lag it.
			Image:        "getmeili/meilisearch:v1.49.0",
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
		d, err := FromJob(j, nil)
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
		d, err := FromJob(j, nil)
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
		d, err := FromJob(j, nil)
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
		d, err := FromJob(j, nil)
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
		d, err := FromJob(j, nil)
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

// TestSearchQFieldsRestrictsMatchingOnRealEngine reproduces the motivating example
// from issue #2671 against a real Meilisearch instance (not a fake): "systems"
// appears only in a company name, not in that job's title, and an unscoped query
// wrongly surfaces it for a caller looking for systems engineering roles.
func TestSearchQFieldsRestrictsMatchingOnRealEngine(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	jobs := []db.Job{
		{
			ID: 20, Title: "Engineer – Asset Manager", Company: "STS Systems Defense", Location: "Tyndall AFB, US",
			Description: "Asset management support role.",
			PublicSlug:  "engineer-asset-manager-sts-aaa",
			PostedAt:    pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment:  enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 21, Title: "Systems Engineer", Company: "Acme", Location: "Berlin",
			Description: "Design and operate distributed systems.",
			PublicSlug:  "systems-engineer-acme-bbb",
			PostedAt:    pgtype.Timestamptz{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment:  enrichedJSON(t, enrich.Enrichment{}),
		},
	}
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, nil)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	t.Run("unscoped query matches the company-name hit too", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{Query: "systems", Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 2 {
			t.Fatalf("Total = %d, want 2 — both the title match and the company-name match", res.Total)
		}
	})

	t.Run("q_fields=title excludes the company-name-only match", func(t *testing.T) {
		res, err := c.Search(ctx, SearchParams{Query: "systems", QFields: []string{"title"}, Limit: 10})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.Total != 1 || res.Hits[0].PublicSlug != "systems-engineer-acme-bbb" {
			t.Fatalf("hits = %+v, want only the title match", res.Hits)
		}
	})
}
func TestSearchOperatorsOnRealEngine(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	day := func(d int) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	jobs := []db.Job{
		{ID: 31, Source: "a", Title: "Product Owner", Company: "Acme", Description: "We write golang.", PublicSlug: "po-31", PostedAt: day(1)},
		{ID: 32, Source: "b", Title: "Produktchef", Company: "Beta", Description: "Work with a product owner.", PublicSlug: "pk-32", PostedAt: day(2)},
		{ID: 33, Source: "a", Title: "Junior Product Owner", Company: "Gamma", Description: "Entry level.", PublicSlug: "jpo-33", PostedAt: day(3)},
		{ID: 34, Source: "c", Title: "Rust Developer", Company: "Delta", Description: "Some golang too.", PublicSlug: "rust-34", PostedAt: day(4)},
	}
	for i := range jobs {
		jobs[i].Enrichment = enrichedJSON(t, enrich.Enrichment{})
	}
	if err := c.IndexJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}
	slugs := func(res SearchResult) []string {
		out := make([]string, 0, len(res.Hits))
		for _, h := range res.Hits {
			out = append(out, h.PublicSlug)
		}
		return out
	}
	search := func(t *testing.T, p SearchParams) SearchResult {
		t.Helper()
		if p.Limit == 0 {
			p.Limit = 10
		}
		res, err := c.Search(ctx, p)
		if err != nil {
			t.Fatalf("Search(%q): %v", p.Query, err)
		}
		return res
	}

	t.Run("title: excludes a description-only match, minus excludes junior", func(t *testing.T) {
		res := search(t, SearchParams{Query: `title:"product owner" -junior`})
		if got := slugs(res); len(got) != 1 || got[0] != "po-31" {
			t.Fatalf("hits = %v, want [po-31]", got)
		}
	})

	t.Run("OR unions alternatives and returns a doc matched twice once", func(t *testing.T) {
		res := search(t, SearchParams{Query: `title:"product owner" -junior OR produktchef OR golang`, Sort: []string{"posted_at:desc"}})
		if got := slugs(res); !reflect.DeepEqual(got, []string{"rust-34", "pk-32", "po-31"}) || res.Total != 3 {
			t.Fatalf("hits = %v total %d, want [rust-34 pk-32 po-31] newest first, total 3", got, res.Total)
		}
	})

	t.Run("paging walks the merged list", func(t *testing.T) {
		res := search(t, SearchParams{Query: `produktchef OR golang`, Sort: []string{"posted_at:desc"}, Limit: 1, Offset: 1})
		if got := slugs(res); !reflect.DeepEqual(got, []string{"pk-32"}) || res.Total != 3 {
			t.Fatalf("page 2 = %v total %d, want [pk-32] of 3", got, res.Total)
		}
	})

	t.Run("facet counts are per document across alternatives", func(t *testing.T) {
		fr, err := c.FacetCounts(ctx, FacetParams{Query: `produktchef OR golang`, Facets: []string{"source"}})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]int64{"a": 1, "b": 1, "c": 1}
		if !reflect.DeepEqual(fr.Facets["source"], want) || fr.Total != 3 {
			t.Fatalf("source facet = %v total %d, want %v total 3", fr.Facets["source"], fr.Total, want)
		}
	})

	t.Run("disjunctive facets under OR", func(t *testing.T) {
		fr, err := c.DisjunctiveFacetCounts(ctx, `produktchef OR golang`, []FacetReq{{Attr: "source"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if fr.Total != 3 || fr.Facets["source"]["c"] != 1 {
			t.Fatalf("FacetResult = %+v", fr)
		}
	})
}

// The experience bounds and their include-unspecified modifier, against a real
// index: a stated requirement is stored as a number, an unstated one as no
// attribute at all (omitempty), and NOT EXISTS has to find exactly the latter.
func TestIntegration_ExperienceIncludeUnspecified(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// The requirement is a dictionary column folded into the nested enrichment at
	// projection time, so the fixture sets the column, not the blob.
	years := func(n int32) pgtype.Int4 { return pgtype.Int4{Int32: n, Valid: true} }
	empty := enrichedJSON(t, enrich.Enrichment{})
	jobs := []db.Job{
		{ID: 1, Title: "Two years", PublicSlug: "two", ExperienceYearsMin: years(2), Enrichment: empty},
		{ID: 2, Title: "Eight years", PublicSlug: "eight", ExperienceYearsMin: years(8), Enrichment: empty},
		{ID: 3, Title: "Not stated", PublicSlug: "unstated", Enrichment: empty},
	}
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, nil)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	for _, tc := range []struct {
		query string
		want  []int64
	}{
		{"experience_years_max=3", []int64{1}},
		{"experience_years_max=3&experience_include_unspecified=true", []int64{1, 3}},
	} {
		v, _ := url.ParseQuery(tc.query)
		res, err := c.Search(ctx, SearchParams{Filter: FilterFromValues(v), Limit: 10})
		if err != nil {
			t.Fatalf("%s: Search: %v", tc.query, err)
		}
		var got []int64
		for _, h := range res.Hits {
			got = append(got, h.ID)
		}
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: hits = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// The seniority include-unspecified modifier against a real index: a posting with
// no level is stored with no seniority attribute (the projection's omitempty), and
// NOT EXISTS has to find exactly those beside the included levels (freehire#3397).
func TestIntegration_SeniorityIncludeUnspecified(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	empty := enrichedJSON(t, enrich.Enrichment{})
	jobs := []db.Job{
		{ID: 1, Title: "Intern", PublicSlug: "intern", Seniority: "intern", Enrichment: empty},
		{ID: 2, Title: "Senior", PublicSlug: "senior", Seniority: "senior", Enrichment: empty},
		{ID: 3, Title: "No level", PublicSlug: "unlabelled", Enrichment: empty},
	}
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, nil)
		if err != nil {
			t.Fatalf("FromJob: %v", err)
		}
		docs = append(docs, d)
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	for _, tc := range []struct {
		query string
		want  []int64
	}{
		{"seniority=intern", []int64{1}},
		{"seniority=intern&seniority_include_unspecified=true", []int64{1, 3}},
	} {
		v, _ := url.ParseQuery(tc.query)
		res, err := c.Search(ctx, SearchParams{Filter: FilterFromValues(v), Limit: 10})
		if err != nil {
			t.Fatalf("%s: Search: %v", tc.query, err)
		}
		var got []int64
		for _, h := range res.Hits {
			got = append(got, h.ID)
		}
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: hits = %v, want %v", tc.query, got, tc.want)
		}
	}
}
