//go:build integration

// Integration tests for the sitemap read path: the job feed returns the freshest
// open jobs (newest id first, closed excluded); the company slice pages by a slug
// cursor and the boundary query returns the cursor at each Nth row so the company
// sitemap index can enumerate chunks without walking the table. SQL behaviors,
// verifiable only against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"fmt"
	"testing"
)

// seedOpenJob upserts one open job under its own company slug, so ordering by id
// (jobs) and by slug (companies) is deterministic across the seeded set.
func seedOpenJob(ctx context.Context, t *testing.T, q *Queries, n int) Job {
	t.Helper()
	p := ingestParams(fmt.Sprintf("acme:%02d", n), fmt.Sprintf("Job %02d", n))
	p.CompanySlug = fmt.Sprintf("co-%02d", n)
	p.Company = fmt.Sprintf("Co %02d", n)
	j, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("seed job %d: %v", n, err)
	}
	return j
}

func TestJobSitemapFreshest(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	var jobs []Job
	for i := 1; i <= 5; i++ {
		jobs = append(jobs, seedOpenJob(ctx, t, q, i)) // ascending ids
	}
	// A closed job must never appear in the sitemap.
	closed := seedOpenJob(ctx, t, q, 6)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}

	t.Run("returns the freshest open jobs, newest id first", func(t *testing.T) {
		got, err := q.ListJobSitemapFreshest(ctx, 3)
		if err != nil {
			t.Fatalf("ListJobSitemapFreshest: %v", err)
		}
		want := []string{jobs[4].PublicSlug, jobs[3].PublicSlug, jobs[2].PublicSlug}
		if fmt.Sprint(freshestSlugs(got)) != fmt.Sprint(want) {
			t.Fatalf("freshest 3 = %v, want %v", freshestSlugs(got), want)
		}
	})

	t.Run("caps at the limit and excludes closed jobs", func(t *testing.T) {
		got, err := q.ListJobSitemapFreshest(ctx, 100)
		if err != nil {
			t.Fatalf("ListJobSitemapFreshest: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("got %d open jobs, want 5 (%v)", len(got), freshestSlugs(got))
		}
		for _, r := range got {
			if r.PublicSlug == closed.PublicSlug {
				t.Fatalf("closed job %q leaked into sitemap", closed.PublicSlug)
			}
		}
	})
}

func TestCompanySitemapKeyset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for i := 1; i <= 5; i++ {
		seedOpenJob(ctx, t, q, i) // also upserts company co-0i
	}

	t.Run("slice pages by slug cursor", func(t *testing.T) {
		first, err := q.ListCompanySitemap(ctx, ListCompanySitemapParams{AfterSlug: "", BatchSize: 2})
		if err != nil {
			t.Fatalf("ListCompanySitemap: %v", err)
		}
		if len(first) != 2 || first[0].Slug != "co-01" || first[1].Slug != "co-02" {
			t.Fatalf("first page = %v, want [co-01 co-02]", companySlugs(first))
		}
		next, err := q.ListCompanySitemap(ctx, ListCompanySitemapParams{AfterSlug: first[len(first)-1].Slug, BatchSize: 2})
		if err != nil {
			t.Fatalf("ListCompanySitemap next: %v", err)
		}
		if len(next) != 2 || next[0].Slug != "co-03" {
			t.Fatalf("next page = %v, want [co-03 co-04]", companySlugs(next))
		}
	})

	t.Run("boundaries return the slug at each Nth company, excluding the last", func(t *testing.T) {
		got, err := q.CompanySitemapBoundaries(ctx, 2)
		if err != nil {
			t.Fatalf("CompanySitemapBoundaries: %v", err)
		}
		want := []string{"co-02", "co-04"}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("boundaries = %v, want %v", got, want)
		}
	})
}

func freshestSlugs(rows []ListJobSitemapFreshestRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.PublicSlug
	}
	return out
}

func companySlugs(rows []ListCompanySitemapRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Slug
	}
	return out
}
