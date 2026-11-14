//go:build integration

package search

import (
	"context"
	"testing"

	"github.com/meilisearch/meilisearch-go"
	"github.com/strelov1/freehire/internal/job/jobview"
)

func TestIntegration_SetViewCountsUpdatesOnlyIndexedDocuments(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if _, err := c.manager.ExperimentalFeatures().SetEditDocumentsByFunction(true).UpdateWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	docs := []JobDocument{
		{ID: 1, Job: jobview.Job{Title: "Golang engineer", CompanySlug: "acme", PublicSlug: "a", ViewCount: 3}},
		{ID: 2, Job: jobview.Job{Title: "Rust engineer", CompanySlug: "beta", PublicSlug: "b", ViewCount: 5}},
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatal(err)
	}

	if err := c.SetViewCounts(ctx, map[int64]int64{1: 42, 999: 7}); err != nil {
		t.Fatal(err)
	}

	var one, two map[string]any
	if err := c.facet.GetDocumentWithContext(ctx, "1", nil, &one); err != nil {
		t.Fatal(err)
	}
	if one["view_count"] != float64(42) || one["title"] != "Golang engineer" {
		t.Fatalf("doc 1 = %v, want view_count 42 and its title kept", one)
	}
	if err := c.facet.GetDocumentWithContext(ctx, "2", nil, &two); err != nil {
		t.Fatal(err)
	}
	if two["view_count"] != float64(5) {
		t.Fatalf("doc 2 view_count = %v, want 5 untouched", two["view_count"])
	}
	var ghost map[string]any
	err := c.facet.GetDocumentWithContext(ctx, "999", nil, &ghost)
	if me, ok := err.(*meilisearch.Error); !ok || me.StatusCode != 404 {
		t.Fatalf("id 999 must not be created, got doc %v err %v", ghost, err)
	}
}
