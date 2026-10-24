//go:build integration

package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/platform/db"
)

// TestIntegration_DistinctCount proves the count is exact against a real engine.
// The fixture holds more POSTINGS than COMPANIES, so a count that ignored
// `distinct` reports 28, and the facet value cap is lowered below the company
// count, so a count read off the facet distribution would be truncated to 3.
// The request shape itself (page/hitsPerPage, never `limit: 0`, no facets) is
// pinned by TestDistinctCount_PagesWithoutLimitOrFacets: the SDK omits a zero
// limit entirely, and at this fixture's size Meili's estimate happens to be
// exact, so only the large-catalogue trap distinguishes the two here.
func TestIntegration_DistinctCount(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// Seven companies; company i posts i+1 jobs (28 postings), and only the first
	// three carry the "backend" category.
	const companies = 7
	var docs []JobDocument
	id := int64(1)
	for i := 0; i < companies; i++ {
		category := "frontend"
		if i < 3 {
			category = "backend"
		}
		for range i + 1 {
			d, err := FromJob(db.Job{
				ID: id, Title: "Go Engineer", Company: fmt.Sprintf("Co %d", i),
				CompanySlug: fmt.Sprintf("co-%d", i), PublicSlug: fmt.Sprintf("j%d", id),
				Category: category,
			}, nil)
			if err != nil {
				t.Fatalf("FromJob: %v", err)
			}
			docs = append(docs, d)
			id++
		}
	}
	if err := c.IndexJobs(ctx, docs); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	// A facet value cap below the company count: the distribution cannot hold them all.
	task, err := c.facet.UpdateFacetingWithContext(ctx, &meilisearch.Faceting{
		MaxValuesPerFacet: 3,
		SortFacetValuesBy: map[string]meilisearch.SortFacetType{"*": meilisearch.SortFacetTypeCount},
	})
	if err != nil {
		t.Fatalf("UpdateFaceting: %v", err)
	}
	if _, err := c.facet.WaitForTaskWithContext(ctx, task.TaskUID, 50*time.Millisecond); err != nil {
		t.Fatalf("await faceting: %v", err)
	}

	t.Run("exact above the facet cap", func(t *testing.T) {
		fr, err := c.FacetCounts(ctx, FacetParams{Facets: []string{"company_slug"}})
		if err != nil {
			t.Fatalf("FacetCounts: %v", err)
		}
		if got := len(fr.Facets["company_slug"]); got != 3 {
			t.Fatalf("precondition: the capped distribution should hold 3 companies, got %d", got)
		}
		if fr.Total != 28 {
			t.Fatalf("precondition: 28 postings, got %d", fr.Total)
		}

		n, err := c.DistinctCount(ctx, "", nil, "company_slug")
		if err != nil {
			t.Fatalf("DistinctCount: %v", err)
		}
		if n != companies {
			t.Errorf("DistinctCount = %d, want %d (28 would mean distinct was ignored)", n, companies)
		}
	})

	t.Run("under a filter and a query", func(t *testing.T) {
		n, err := c.DistinctCount(ctx, "engineer", Filter([]string{Eq("enrichment.category", "backend")}), "company_slug")
		if err != nil {
			t.Fatalf("DistinctCount: %v", err)
		}
		if n != 3 {
			t.Errorf("DistinctCount(backend) = %d, want 3", n)
		}
	})

	t.Run("an OR query is refused rather than over-counted", func(t *testing.T) {
		_, err := c.DistinctCount(ctx, "go OR engineer", nil, "company_slug")
		if err == nil {
			t.Fatal("DistinctCount over an OR query should fail, it cannot be exact")
		}
	})
}
