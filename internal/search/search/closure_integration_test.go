//go:build integration

package search

import (
	"context"
	"reflect"
	"testing"

	"github.com/meilisearch/meilisearch-go"
	"github.com/strelov1/freehire/internal/job/jobview"
)

func TestIntegration_ClosureUpdateHidesWithoutDeletingOrCreatingDocuments(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if _, err := c.manager.ExperimentalFeatures().SetEditDocumentsByFunction(true).UpdateWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	docs := []JobDocument{
		{ID: 1, Job: jobview.Job{Title: "Golang engineer", Company: "Acme", CompanySlug: "acme", PublicSlug: "closed-job"}},
		{ID: 2, Job: jobview.Job{Title: "Golang engineer", Company: "Beta", CompanySlug: "beta", PublicSlug: "open-job"}},
	}
	for i := range docs {
		docs[i].Reality = &jobview.Reality{Class: "fresh"}
	}
	docs[0].Vectors = DocumentVectors{Skills: make([]float32, 1280), Desc: [][]float32{make([]float32, 768)}}
	docs[0].Vectors.Skills[0] = 1
	docs[0].Vectors.Desc[0][0] = 1
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatal(err)
	}
	var before map[string]any
	if err := c.facet.GetDocumentWithContext(ctx, "1", &meilisearch.DocumentQuery{RetrieveVectors: true}, &before); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseJobs(ctx, []int64{1, 999}); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := c.facet.GetDocumentWithContext(ctx, "1", &meilisearch.DocumentQuery{RetrieveVectors: true}, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["is_closed"] != true || raw["title"] != "Golang engineer" {
		t.Fatalf("closure did not preserve the document: %v", raw)
	}
	if !reflect.DeepEqual(raw["_vectors"], before["_vectors"]) {
		t.Fatal("closure update changed stored vectors")
	}
	stats, err := c.facet.GetStatsWithContext(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NumberOfDocuments != 2 {
		t.Fatalf("documents=%d, want 2: missing ids must not create skeletons", stats.NumberOfDocuments)
	}
	for _, query := range []string{"Golang", "Golang OR engineer"} {
		found, err := c.Search(ctx, SearchParams{Query: query, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(found.Hits) != 1 || found.Hits[0].ID != 2 || found.Total != 1 {
			t.Fatalf("query %q returned closed document: %+v", query, found)
		}
		facets, err := c.FacetCounts(ctx, FacetParams{Query: query, Facets: []string{"company_slug"}})
		if err != nil {
			t.Fatal(err)
		}
		if facets.Total != 1 || facets.Facets["company_slug"]["acme"] != 0 || facets.Facets["company_slug"]["beta"] != 1 {
			t.Fatalf("closed job counted in facets: %+v", facets)
		}
	}
	page, total, err := c.ListSitemapPage(ctx, 0, 10)
	if err != nil || total != 1 || len(page) != 1 || page[0].Slug != "open-job" {
		t.Fatalf("closed job remained in sitemap: page=%+v total=%d err=%v", page, total, err)
	}
	count, err := c.DistinctCount(ctx, "Golang", nil, "company_slug")
	if err != nil || count != 1 {
		t.Fatalf("distinct companies=%d err=%v, want 1", count, err)
	}
	if err := c.IndexJobs(ctx, docs[:1]); err != nil {
		t.Fatal(err)
	}
	reopened, err := c.Search(ctx, SearchParams{Query: "Golang", Limit: 10})
	if err != nil || len(reopened.Hits) != 2 {
		t.Fatalf("reopened document stayed hidden: %+v err=%v", reopened, err)
	}
}
