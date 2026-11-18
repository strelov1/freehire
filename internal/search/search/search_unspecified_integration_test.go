//go:build integration

package search

import (
	"context"
	"net/url"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
)

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
		d, err := FromJob(j, "")
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
		d, err := FromJob(j, "")
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

// The entry-level profile alert's rule against a real index: "intern, or no level and
// asking for at most a year". A labelled internship stating no years must pass, which
// is the posting the bound alone dropped (freehire#3655), while an unlabelled posting
// asking for five years and a labelled senior one must not.
func TestIntegration_ExperienceOnlySeniorityUnspecified(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	years := func(n int32) pgtype.Int4 { return pgtype.Int4{Int32: n, Valid: true} }
	empty := enrichedJSON(t, enrich.Enrichment{})
	jobs := []db.Job{
		{ID: 1, Title: "Intern, no years", PublicSlug: "intern-unstated", Seniority: "intern", Enrichment: empty},
		{ID: 2, Title: "Intern, three years", PublicSlug: "intern-three", Seniority: "intern", ExperienceYearsMin: years(3), Enrichment: empty},
		{ID: 3, Title: "No level, one year", PublicSlug: "unlabelled-one", ExperienceYearsMin: years(1), Enrichment: empty},
		{ID: 4, Title: "No level, five years", PublicSlug: "unlabelled-five", ExperienceYearsMin: years(5), Enrichment: empty},
		{ID: 5, Title: "No level, no years", PublicSlug: "unlabelled-unstated", Enrichment: empty},
		{ID: 6, Title: "Senior, one year", PublicSlug: "senior-one", Seniority: "senior", ExperienceYearsMin: years(1), Enrichment: empty},
	}
	docs := make([]JobDocument, 0, len(jobs))
	for _, j := range jobs {
		d, err := FromJob(j, "")
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
		{"seniority=intern&seniority_include_unspecified=true&experience_years_max=1", []int64{3}},
		{"seniority=intern&seniority_include_unspecified=true&experience_years_max=1&experience_only_seniority_unspecified=true", []int64{1, 2, 3}},
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
