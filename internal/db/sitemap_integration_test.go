//go:build integration

// Integration tests for the sitemap keyset read path: the slim slice queries page
// by an id/slug cursor (never OFFSET) and skip closed jobs, and the boundary
// queries return the cursor at each Nth row so the sitemap index can enumerate
// chunks without walking the catalogue. SQL behaviors, verifiable only against a
// real Postgres. Run with: go test -tags=integration ./internal/db/
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

func TestJobSitemapKeyset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	var jobs []Job
	for i := 1; i <= 5; i++ {
		jobs = append(jobs, seedOpenJob(ctx, t, q, i))
	}
	// A closed job must never appear in the sitemap slice nor shift the cursor math.
	closed := seedOpenJob(ctx, t, q, 6)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}

	t.Run("slice pages by id cursor and skips closed jobs", func(t *testing.T) {
		first, err := q.ListJobSitemap(ctx, ListJobSitemapParams{AfterID: 0, BatchSize: 2})
		if err != nil {
			t.Fatalf("ListJobSitemap: %v", err)
		}
		if len(first) != 2 || first[0].ID != jobs[0].ID || first[1].ID != jobs[1].ID {
			t.Fatalf("first page = %v, want [%d %d]", jobSitemapIDs(first), jobs[0].ID, jobs[1].ID)
		}
		if first[0].PublicSlug != jobs[0].PublicSlug {
			t.Fatalf("public_slug = %q, want %q", first[0].PublicSlug, jobs[0].PublicSlug)
		}

		// Drain the rest via the keyset cursor: it must stop at the 5 open jobs and
		// never surface the closed one.
		var seen []int64
		cursor := int64(0)
		for {
			page, err := q.ListJobSitemap(ctx, ListJobSitemapParams{AfterID: cursor, BatchSize: 2})
			if err != nil {
				t.Fatalf("ListJobSitemap page: %v", err)
			}
			if len(page) == 0 {
				break
			}
			for _, r := range page {
				seen = append(seen, r.ID)
			}
			cursor = page[len(page)-1].ID
		}
		if len(seen) != 5 {
			t.Fatalf("drained %d open jobs, want 5 (ids %v)", len(seen), seen)
		}
		for _, id := range seen {
			if id == closed.ID {
				t.Fatalf("closed job %d leaked into sitemap slice", closed.ID)
			}
		}
	})

	t.Run("boundaries return the id at each Nth open job, excluding the last", func(t *testing.T) {
		// 5 open jobs, chunk size 2 -> cursors after rows 2 and 4 (row 6=EOF excluded).
		got, err := q.JobSitemapBoundaries(ctx, 2)
		if err != nil {
			t.Fatalf("JobSitemapBoundaries: %v", err)
		}
		want := []int64{jobs[1].ID, jobs[3].ID}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("boundaries = %v, want %v", got, want)
		}
	})

	t.Run("boundaries empty when a single chunk covers everything", func(t *testing.T) {
		got, err := q.JobSitemapBoundaries(ctx, 50)
		if err != nil {
			t.Fatalf("JobSitemapBoundaries: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("boundaries = %v, want none", got)
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

func jobSitemapIDs(rows []ListJobSitemapRow) []int64 {
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.ID
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
