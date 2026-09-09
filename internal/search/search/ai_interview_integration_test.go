//go:build integration

// Integration tests for the ai_interview facet against a real Meilisearch. The
// behaviour worth proving is the negative: nothing in the index is ever written
// false, so "hide these employers" has to be NOT of the positive, and an equality on
// false would silently return nothing. Run with:
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

func TestSearchFiltersByAIInterviewFacet(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	posted := pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	jobs := []db.Job{
		{
			ID: 40, Title: "AI Trainer", Company: "Botco", Location: "Remote",
			PublicSlug: "ai-trainer-botco-aaa", CompanySlug: "botco",
			AiInterviewReports: 3,
			PostedAt:           posted,
			Enrichment:         enrichedJSON(t, enrich.Enrichment{}),
		},
		{
			ID: 41, Title: "Backend Engineer", Company: "Humanco", Location: "Remote",
			PublicSlug: "backend-engineer-humanco-bbb", CompanySlug: "humanco",
			PostedAt:   posted,
			Enrichment: enrichedJSON(t, enrich.Enrichment{}),
		},
	}
	if err := c.IndexJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}

	// The positive selects the reported employer.
	res, err := c.Search(ctx, SearchParams{
		Filter: FilterFromValues(url.Values{AIInterviewParam: {"true"}}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("search ai_interview=true: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "ai-trainer-botco-aaa" {
		t.Fatalf("ai_interview=true hits = %+v, want only the reported employer", res.Hits)
	}

	// The negative is what the feature is for, and it must reach the documents that
	// omit the attribute entirely — every posting whose company nobody has reported.
	res, err = c.Search(ctx, SearchParams{
		Filter: FilterFromValues(url.Values{AIInterviewParam: {"false"}}),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("search ai_interview=false: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].PublicSlug != "backend-engineer-humanco-bbb" {
		t.Fatalf("ai_interview=false hits = %+v, want only the unreported employer", res.Hits)
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

// The param must be part of the filter vocabulary, or an endpoint that widens when it
// does not understand a param would silently return the employers the caller asked to
// hide — and say nothing.
func TestAIInterviewParamIsKnown(t *testing.T) {
	unknown := UnknownParams(url.Values{AIInterviewParam: {"false"}}, nil)
	if len(unknown) != 0 {
		t.Fatalf("%s reported as unknown: %+v", AIInterviewParam, unknown)
	}
}
