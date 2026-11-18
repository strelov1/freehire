//go:build integration

package search

import (
	"context"
	"maps"
	"reflect"
	"testing"

	"github.com/meilisearch/meilisearch-go"
	"github.com/strelov1/freehire/internal/job/jobview"
)

func TestIntegration_VisibilityUpdatesRetainContentAndHideFromAllReaders(t *testing.T) {
	for _, status := range []struct {
		marker string
		mark   func(*Client, context.Context, []int64) error
	}{
		{marker: "is_closed", mark: (*Client).CloseJobs},
		{marker: "is_duplicate", mark: (*Client).MarkDuplicateJobs},
	} {
		t.Run(status.marker, func(t *testing.T) {
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
			docs[0].Vectors = DocumentVectors{Skills: make([]float32, 1280)}
			docs[0].Vectors.Skills[0] = 1
			docs[1].Vectors = docs[0].Vectors
			if err := c.IndexJobs(ctx, docs); err != nil {
				t.Fatal(err)
			}
			// Retired desc vectors remain in legacy documents after the embedder is
			// removed. Their nested array total can exceed Rhai's 10,000-item limit.
			legacyVectors := make([][]float32, 14)
			for i := range legacyVectors {
				legacyVectors[i] = make([]float32, 768)
				legacyVectors[i][i] = 1
			}
			task, err := c.facet.UpdateDocumentsWithContext(ctx, []map[string]any{{
				"id": 1,
				"_vectors": map[string]any{
					"skills": docs[0].Vectors.Skills,
					"desc":   map[string]any{"embeddings": legacyVectors, "regenerate": false},
				},
			}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.awaitTask(ctx, c.facet, task.TaskUID); err != nil {
				t.Fatal(err)
			}
			var before map[string]any
			if err := c.facet.GetDocumentWithContext(ctx, "1", &meilisearch.DocumentQuery{RetrieveVectors: true}, &before); err != nil {
				t.Fatal(err)
			}
			if before["_vectors"].(map[string]any)["desc"] == nil {
				t.Fatal("legacy desc vectors were not stored")
			}
			if err := status.mark(c, ctx, []int64{1, 999}); err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := c.facet.GetDocumentWithContext(ctx, "1", &meilisearch.DocumentQuery{RetrieveVectors: true}, &raw); err != nil {
				t.Fatal(err)
			}
			if raw[status.marker] != true || raw["title"] != "Golang engineer" {
				t.Fatalf("status update did not preserve the document: %v", raw)
			}
			vectorsBefore := before["_vectors"].(map[string]any)
			vectorsAfter := raw["_vectors"].(map[string]any)
			if !reflect.DeepEqual(vectorsAfter["skills"], vectorsBefore["skills"]) {
				t.Fatal("status update changed active skills vectors")
			}
			if _, ok := vectorsAfter["desc"]; ok {
				t.Fatal("status update retained retired desc vectors")
			}
			delete(vectorsBefore, "desc")
			content := maps.Clone(raw)
			delete(content, status.marker)
			if !reflect.DeepEqual(content, before) {
				t.Fatal("status update changed content beyond its marker and retired vectors")
			}
			if err := status.mark(c, ctx, []int64{1, 999}); err != nil {
				t.Fatalf("repeated status update: %v", err)
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
					t.Fatalf("query %q returned hidden document: %+v", query, found)
				}
				facets, err := c.FacetCounts(ctx, FacetParams{Query: query, Facets: []string{"company_slug"}})
				if err != nil {
					t.Fatal(err)
				}
				if facets.Total != 1 || facets.Facets["company_slug"]["acme"] != 0 || facets.Facets["company_slug"]["beta"] != 1 {
					t.Fatalf("hidden job counted in facets: %+v", facets)
				}
			}
			found, err := c.Search(ctx, SearchParams{Query: "Golang", Vector: docs[0].Vectors.Skills, Limit: 10})
			if err != nil || len(found.Hits) != 1 || found.Hits[0].ID != 2 {
				t.Fatalf("marked document reached vector search: %+v err=%v", found, err)
			}
			page, total, err := c.ListSitemapPage(ctx, 0, 10)
			if err != nil || total != 1 || len(page) != 1 || page[0].Slug != "open-job" {
				t.Fatalf("hidden job remained in sitemap: page=%+v total=%d err=%v", page, total, err)
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
			if err := status.mark(c, ctx, []int64{2}); err != nil {
				t.Fatalf("status update without retired vectors: %v", err)
			}
			modern, err := c.Search(ctx, SearchParams{Query: "Golang", Limit: 10})
			if err != nil || len(modern.Hits) != 1 || modern.Hits[0].ID != 1 {
				t.Fatalf("document without retired vectors stayed visible: %+v err=%v", modern, err)
			}
		})
	}
}
