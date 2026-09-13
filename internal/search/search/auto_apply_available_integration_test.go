//go:build integration

// Integration tests for the auto_apply_available facet against a real
// Meilisearch. Mirrors ai_interview_integration_test.go: the behaviour worth
// proving is the negative, since nothing in the index is ever written false,
// so excluding eligible postings has to be NOT of the positive. Run with:
//
//	go test -tags=integration ./internal/search/search/
package search

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestSearchFiltersByAutoApplyAvailableFacet(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	posted := pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	jobs := []db.Job{
		{
			ID: 50, Title: "Backend Engineer", Company: "Greenco", Location: "Remote",
			PublicSlug: "backend-engineer-greenco-ccc", CompanySlug: "greenco",
			Source:     "greenhouse",
			PostedAt:   posted,
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 51, Title: "Backend Engineer", Company: "Recruiteeco", Location: "Remote",
			PublicSlug: "backend-engineer-recruiteeco-ddd", CompanySlug: "recruiteeco",
			Source:     "recruitee",
			PostedAt:   posted,
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}
	if err := c.IndexJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	// The positive selects only the eligible-provider posting.
	res, err := c.Search(ctx, SearchParams{
		Filter: FilterFromValues(url.Values{AutoApplyAvailableParam: {"true"}}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("search auto_apply_available=true: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "backend-engineer-greenco-ccc" {
		t.Fatalf("auto_apply_available=true hits = %+v, want only the eligible-provider posting", res.Hits)
	}

	// The negative must reach the documents that omit the attribute entirely —
	// every posting whose provider auto-apply cannot currently drive.
	res, err = c.Search(ctx, SearchParams{
		Filter: FilterFromValues(url.Values{AutoApplyAvailableParam: {"false"}}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("search auto_apply_available=false: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "backend-engineer-recruiteeco-ddd" {
		t.Fatalf("auto_apply_available=false hits = %+v, want only the ineligible-provider posting", res.Hits)
	}

	// Omitting the filter returns both: the facet narrows, it never applies itself.
	res, err = c.Search(ctx, SearchParams{Limit: 10})
	if err != nil {
		t.Fatalf("search without the filter: %v", err)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("unfiltered hits = %d, want 2", len(res.Hits))
	}
}

// The param must be part of the filter vocabulary, or an endpoint that widens
// when it does not understand a param would silently return every provider —
// including the ones auto-apply cannot drive — and say nothing.
func TestAutoApplyAvailableParamIsKnown(t *testing.T) {
	unknown := UnknownParams(url.Values{AutoApplyAvailableParam: {"true"}}, nil)
	if len(unknown) != 0 {
		t.Fatalf("%s reported as unknown: %+v", AutoApplyAvailableParam, unknown)
	}
}
