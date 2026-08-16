//go:build integration

// Integration tests for the COMPANY sitemap read path: the slice pages by a slug
// keyset cursor, and the boundary query returns the cursor at each Nth row so the
// sitemap index can enumerate chunks without walking the table. SQL behaviors,
// verifiable only against a real Postgres. Run with: go test -tags=integration ./internal/db/
//
// The job sitemap has no SQL to test — it pages the Meilisearch index instead (see
// internal/search/sitemap.go), which is why only companies are covered here.
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

func companySlugs(rows []ListCompanySitemapRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Slug
	}
	return out
}
