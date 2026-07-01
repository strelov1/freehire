//go:build integration

// Integration tests for the denormalized companies.job_count: RefreshCompanyFacets
// recomputes each company's OPEN-job count (closed_at IS NULL) in one set-based pass
// and zeroes a company whose jobs all closed, and ListCompanies orders by that count
// descending (tie-broken by name) while still honoring the name filter. This is SQL
// behavior, so it runs against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// closeJobByExtID marks a job closed, as the ingest sweep / liveness worker would.
func closeJobByExtID(t *testing.T, pool *pgxpool.Pool, externalID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE jobs SET closed_at = now() WHERE external_id = $1`, externalID); err != nil {
		t.Fatalf("close job %q: %v", externalID, err)
	}
}

// companyJobCount reads the denormalized counter straight from the row.
func companyJobCount(t *testing.T, pool *pgxpool.Pool, slug string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT job_count FROM companies WHERE slug = $1`, slug).Scan(&n); err != nil {
		t.Fatalf("read job_count %q: %v", slug, err)
	}
	return n
}

func TestRefreshCompanyFacetsJobCount(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertCompany(t, pool, "acme", "Acme Corp")
	insertCompany(t, pool, "globex", "Globex")
	insertCompany(t, pool, "empty-co", "Empty Co")

	// Acme: 3 open + 2 closed. Globex: 1 open. Empty Co: none.
	insertJobForCompany(t, pool, "acme:1", "acme")
	insertJobForCompany(t, pool, "acme:2", "acme")
	insertJobForCompany(t, pool, "acme:3", "acme")
	insertJobForCompany(t, pool, "acme:c1", "acme")
	insertJobForCompany(t, pool, "acme:c2", "acme")
	closeJobByExtID(t, pool, "acme:c1")
	closeJobByExtID(t, pool, "acme:c2")
	insertJobForCompany(t, pool, "globex:1", "globex")

	t.Run("counts only open jobs", func(t *testing.T) {
		if _, err := q.RefreshCompanyFacets(ctx); err != nil {
			t.Fatalf("recount: %v", err)
		}
		if got := companyJobCount(t, pool, "acme"); got != 3 {
			t.Errorf("acme job_count = %d, want 3 (open only, closed excluded)", got)
		}
		if got := companyJobCount(t, pool, "globex"); got != 1 {
			t.Errorf("globex job_count = %d, want 1", got)
		}
		if got := companyJobCount(t, pool, "empty-co"); got != 0 {
			t.Errorf("empty-co job_count = %d, want 0", got)
		}
	})

	t.Run("zeroes a company whose jobs all closed", func(t *testing.T) {
		closeJobByExtID(t, pool, "acme:1")
		closeJobByExtID(t, pool, "acme:2")
		closeJobByExtID(t, pool, "acme:3")
		if _, err := q.RefreshCompanyFacets(ctx); err != nil {
			t.Fatalf("recount: %v", err)
		}
		if got := companyJobCount(t, pool, "acme"); got != 0 {
			t.Errorf("acme job_count = %d after all jobs closed, want 0", got)
		}
	})

	t.Run("re-running rewrites nothing", func(t *testing.T) {
		rows, err := q.RefreshCompanyFacets(ctx)
		if err != nil {
			t.Fatalf("recount: %v", err)
		}
		if rows != 0 {
			t.Errorf("idempotent recount affected %d rows, want 0", rows)
		}
	})
}

func TestListCompaniesOrdersByJobCount(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertCompany(t, pool, "alpha", "Alpha")
	insertCompany(t, pool, "beta", "Beta")
	insertCompany(t, pool, "gamma", "Gamma")

	// Beta and Gamma each have 3 open jobs; Alpha has 1. Expected order:
	// Beta, Gamma (count tie broken by name), then Alpha.
	for _, ext := range []string{"beta:1", "beta:2", "beta:3"} {
		insertJobForCompany(t, pool, ext, "beta")
	}
	for _, ext := range []string{"gamma:1", "gamma:2", "gamma:3"} {
		insertJobForCompany(t, pool, ext, "gamma")
	}
	insertJobForCompany(t, pool, "alpha:1", "alpha")
	if _, err := q.RefreshCompanyFacets(ctx); err != nil {
		t.Fatalf("recount: %v", err)
	}

	t.Run("most active first, ties by name", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Search: "", Limit: 50, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		gotSlugs := make([]string, len(rows))
		for i, r := range rows {
			gotSlugs[i] = r.Slug
		}
		want := []string{"beta", "gamma", "alpha"}
		if len(gotSlugs) != len(want) {
			t.Fatalf("got %v, want %v", gotSlugs, want)
		}
		for i := range want {
			if gotSlugs[i] != want[i] {
				t.Fatalf("order = %v, want %v", gotSlugs, want)
			}
		}
		if rows[0].JobCount != 3 || rows[2].JobCount != 1 {
			t.Errorf("job counts not surfaced: %+v", rows)
		}
	})

	t.Run("name filter still applies", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Search: "al", Limit: 50, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(rows) != 1 || rows[0].Slug != "alpha" {
			t.Fatalf("search 'al' returned %+v, want only alpha", rows)
		}
	})
}
