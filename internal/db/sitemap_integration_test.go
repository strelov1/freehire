//go:build integration

// Integration tests for the sitemap read path: both the job and company slices page
// by a keyset cursor (id and slug respectively), and each boundary query returns the
// cursor at each Nth row so the sitemap index can enumerate chunks without walking
// the table. The tiling test is the one that matters — chunks must join up exactly,
// serving no row twice and skipping none between files. SQL behaviors,
// verifiable only against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"fmt"
	"math"
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

func TestJobSitemapKeyset(t *testing.T) {
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

	// firstChunk is the cursor the handler opens the first chunk with: above every id.
	const firstChunk = int64(math.MaxInt64)

	t.Run("first chunk returns the newest jobs, newest id first", func(t *testing.T) {
		got, err := q.ListJobSitemapChunk(ctx, ListJobSitemapChunkParams{AfterID: firstChunk, RowLimit: 3})
		if err != nil {
			t.Fatalf("ListJobSitemapChunk: %v", err)
		}
		want := []string{jobs[4].PublicSlug, jobs[3].PublicSlug, jobs[2].PublicSlug}
		if fmt.Sprint(freshestSlugs(got)) != fmt.Sprint(want) {
			t.Fatalf("first chunk = %v, want %v", freshestSlugs(got), want)
		}
	})

	t.Run("caps at the limit and excludes closed jobs", func(t *testing.T) {
		got, err := q.ListJobSitemapChunk(ctx, ListJobSitemapChunkParams{AfterID: firstChunk, RowLimit: 100})
		if err != nil {
			t.Fatalf("ListJobSitemapChunk: %v", err)
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

	// The property the whole chunked scheme rests on: following a boundary cursor
	// picks up exactly where the previous chunk stopped — no row served twice, none
	// skipped between files.
	t.Run("chunks tile the catalogue without gaps or repeats", func(t *testing.T) {
		cursors, err := q.JobSitemapBoundaries(ctx, 2)
		if err != nil {
			t.Fatalf("JobSitemapBoundaries: %v", err)
		}
		var seen []string
		for _, after := range append([]int64{firstChunk}, cursors...) {
			page, err := q.ListJobSitemapChunk(ctx, ListJobSitemapChunkParams{AfterID: after, RowLimit: 2})
			if err != nil {
				t.Fatalf("chunk after %d: %v", after, err)
			}
			seen = append(seen, freshestSlugs(page)...)
		}
		want := []string{
			jobs[4].PublicSlug, jobs[3].PublicSlug, jobs[2].PublicSlug,
			jobs[1].PublicSlug, jobs[0].PublicSlug,
		}
		if fmt.Sprint(seen) != fmt.Sprint(want) {
			t.Fatalf("walked %v, want %v", seen, want)
		}
	})

	t.Run("boundaries return the id at each Nth job, excluding the last", func(t *testing.T) {
		got, err := q.JobSitemapBoundaries(ctx, 2)
		if err != nil {
			t.Fatalf("JobSitemapBoundaries: %v", err)
		}
		want := []int64{jobs[3].ID, jobs[1].ID}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("boundaries = %v, want %v", got, want)
		}
	})

	// The chunk size divides the open count exactly, so the final row lands ON a
	// chunk edge — its cursor would open a sub-sitemap with no URLs in it.
	t.Run("boundaries exclude a final row that lands on a chunk edge", func(t *testing.T) {
		got, err := q.JobSitemapBoundaries(ctx, 5)
		if err != nil {
			t.Fatalf("JobSitemapBoundaries: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("boundaries = %v, want none (the 5th job is also the last)", got)
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
	// co-06 keeps a job, but a closed one — so it is a company with nothing open,
	// the ~90k-row shape (job-less reference imports) the sitemap must not list.
	jobless := seedOpenJob(ctx, t, q, 6)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, jobless.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}
	// companies.job_count is denormalized, recomputed by cmd/recount-companies rather
	// than on the write path, and the sitemap filters on it — so the seed is only in
	// its final state once the recount has run.
	if _, err := q.RefreshCompanyFacets(ctx); err != nil {
		t.Fatalf("RefreshCompanyFacets: %v", err)
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

	// The chunk size divides the hiring count exactly, so the final row IS a boundary
	// — the case the max(slug) guard replaced `rn < total` to keep excluding. Its
	// cursor would open a sub-sitemap with no URLs in it.
	t.Run("boundaries exclude a final row that lands on a chunk edge", func(t *testing.T) {
		got, err := q.CompanySitemapBoundaries(ctx, 5)
		if err != nil {
			t.Fatalf("CompanySitemapBoundaries: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("boundaries = %v, want none (co-05 is both the 5th and the last)", got)
		}
	})

	t.Run("a company with no open job is listed nowhere", func(t *testing.T) {
		page, err := q.ListCompanySitemap(ctx, ListCompanySitemapParams{AfterSlug: "co-04", BatchSize: 50})
		if err != nil {
			t.Fatalf("ListCompanySitemap: %v", err)
		}
		if fmt.Sprint(companySlugs(page)) != fmt.Sprint([]string{"co-05"}) {
			t.Fatalf("tail page = %v, want [co-05] — co-06 has nothing open", companySlugs(page))
		}
	})
}

func freshestSlugs(rows []ListJobSitemapChunkRow) []string {
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
