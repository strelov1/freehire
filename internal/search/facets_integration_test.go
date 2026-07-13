//go:build integration

// Integration test for FacetCounts against a real Meilisearch (testcontainers):
// it exercises the actual facetDistribution/facetStats round-trip the unit tests
// can only stub. Run with:
//
//	go test -tags=integration ./internal/search/
//
// Requires Docker; the first run is slow (the embedder downloads its model when
// EnsureIndex applies the index settings).
package search

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

func TestIntegration_FacetCounts(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	money := func(n int) *int { return &n }
	jobs := []db.Job{
		{
			ID: 1, Title: "Senior Go Engineer", Company: "Acme", PublicSlug: "a",
			Skills: []string{"go"},
			// seniority/category are dictionary columns (served dict-only); salary
			// stays LLM-only in the enrichment payload.
			Seniority:  "senior",
			Category:   "backend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{SalaryMin: money(100000), SalaryMax: money(150000)}),
		},
		{
			ID: 2, Title: "Senior Backend Dev", Company: "Beta", PublicSlug: "b",
			Skills:     []string{"go", "kubernetes"},
			Seniority:  "senior",
			Category:   "backend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{SalaryMin: money(120000), SalaryMax: money(200000)}),
		},
		{
			ID: 3, Title: "Junior Frontend Dev", Company: "Gamma", PublicSlug: "c",
			Skills:     []string{"react"},
			Seniority:  "junior",
			Category:   "frontend",
			PostedAt:   pgtype.Timestamptz{Time: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			Enrichment: enrichedJSON(t, enrich.Enrichment{SalaryMin: money(50000), SalaryMax: money(70000)}),
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

	attrs := []string{"enrichment.seniority", "enrichment.category", "skills", "enrichment.salary_min", "enrichment.salary_max"}

	t.Run("distribution and stats over the whole set", func(t *testing.T) {
		res, err := c.FacetCounts(ctx, FacetParams{Facets: attrs})
		if err != nil {
			t.Fatalf("FacetCounts: %v", err)
		}
		if res.Total != 3 {
			t.Errorf("Total = %d, want 3", res.Total)
		}
		if res.Facets["enrichment.seniority"]["senior"] != 2 || res.Facets["enrichment.seniority"]["junior"] != 1 {
			t.Errorf("seniority dist = %v", res.Facets["enrichment.seniority"])
		}
		if res.Facets["enrichment.category"]["backend"] != 2 {
			t.Errorf("category dist = %v", res.Facets["enrichment.category"])
		}
		if res.Facets["skills"]["go"] != 2 {
			t.Errorf("skills dist = %v", res.Facets["skills"])
		}
		if res.Stats["enrichment.salary_min"].Min != 50000 {
			t.Errorf("salary_min stat = %+v, want min 50000", res.Stats["enrichment.salary_min"])
		}
		if res.Stats["enrichment.salary_max"].Max != 200000 {
			t.Errorf("salary_max stat = %+v, want max 200000", res.Stats["enrichment.salary_max"])
		}
	})

	t.Run("filter narrows the counts and stats", func(t *testing.T) {
		res, err := c.FacetCounts(ctx, FacetParams{
			Facets: attrs,
			Filter: Filter([]string{Eq("enrichment.category", "backend")}),
		})
		if err != nil {
			t.Fatalf("FacetCounts: %v", err)
		}
		if res.Total != 2 {
			t.Errorf("filtered Total = %d, want 2", res.Total)
		}
		if res.Facets["enrichment.seniority"]["senior"] != 2 {
			t.Errorf("filtered seniority dist = %v", res.Facets["enrichment.seniority"])
		}
		if _, ok := res.Facets["enrichment.seniority"]["junior"]; ok {
			t.Errorf("junior should not appear under category=backend: %v", res.Facets["enrichment.seniority"])
		}
		if res.Stats["enrichment.salary_min"].Min != 100000 {
			t.Errorf("filtered salary_min stat = %+v, want min 100000", res.Stats["enrichment.salary_min"])
		}
	})
}

// TestIntegration_IsTechFilter locks the is_tech facet spec scenarios against a real
// Meilisearch: the distribution reports the tech/non_tech buckets (an unknown/NULL
// is_tech job is absent, since jobview omits it), and filtering to tech excludes both
// the non_tech and the unknown jobs. The is_tech column is set on the fixtures the way
// UpsertJob persists it (the derivation itself is unit-tested); this covers the
// index/filter round-trip the config-level unit tests cannot.
func TestIntegration_IsTechFilter(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	yes := pgtype.Bool{Bool: true, Valid: true}
	no := pgtype.Bool{Bool: false, Valid: true}
	unknown := pgtype.Bool{} // NULL — jobview omits the facet
	jobs := []db.Job{
		{ID: 1, Title: "Senior Go Engineer", Company: "Acme", PublicSlug: "a", Category: "backend", IsTech: yes},
		{ID: 2, Title: "Warehouse Cleaner", Company: "Beta", PublicSlug: "b", IsTech: no},
		{ID: 3, Title: "Yard Coordinator", Company: "Gamma", PublicSlug: "c", IsTech: unknown},
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

	t.Run("distribution reports tech and non_tech, unknown absent", func(t *testing.T) {
		res, err := c.FacetCounts(ctx, FacetParams{Facets: []string{"is_tech"}})
		if err != nil {
			t.Fatalf("FacetCounts: %v", err)
		}
		if res.Facets["is_tech"]["tech"] != 1 || res.Facets["is_tech"]["non_tech"] != 1 {
			t.Errorf("is_tech dist = %v, want tech:1 non_tech:1", res.Facets["is_tech"])
		}
		if len(res.Facets["is_tech"]) != 2 {
			t.Errorf("is_tech dist should have no unknown bucket, got %v", res.Facets["is_tech"])
		}
	})

	t.Run("filter to tech excludes non_tech and unknown", func(t *testing.T) {
		res, err := c.FacetCounts(ctx, FacetParams{
			Facets: []string{"is_tech"},
			Filter: Filter([]string{Eq("is_tech", "tech")}),
		})
		if err != nil {
			t.Fatalf("FacetCounts: %v", err)
		}
		if res.Total != 1 {
			t.Errorf("filtered Total = %d, want 1 (only the tech job)", res.Total)
		}
	})
}
