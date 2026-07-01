//go:build integration

// Integration tests for the denormalized company facet arrays: RefreshCompanyFacets
// aggregates each company's regions/countries (from the jobs geography columns) and
// domains/company_types/company_sizes (from the jobs.enrichment JSONB) as the
// distinct union over its OPEN jobs (closed_at IS NULL), in the same set-based pass
// that maintains job_count. This is SQL behavior (array_agg over unnest /
// jsonb_array_elements_text + the IS DISTINCT FROM guard), so it runs against a real
// Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"slices"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// insertJobWithFacets seeds a job carrying geography columns and an enrichment blob,
// so the recompute has something to aggregate. An empty enrichment ("{}") models an
// unenriched job.
func insertJobWithFacets(t *testing.T, pool *pgxpool.Pool, externalID, companySlug string, regions, countries []string, enrichment string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, regions, countries, enrichment)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3, $4, $5)`,
		externalID, companySlug, regions, countries, enrichment); err != nil {
		t.Fatalf("insert job %q: %v", externalID, err)
	}
}

// companyTextArray reads one denormalized facet array off the company row, sorted so
// assertions are order-independent.
func companyTextArray(t *testing.T, pool *pgxpool.Pool, slug, column string) []string {
	t.Helper()
	var got []string
	if err := pool.QueryRow(context.Background(),
		`SELECT `+column+` FROM companies WHERE slug = $1`, slug).Scan(&got); err != nil {
		t.Fatalf("read %s %q: %v", column, slug, err)
	}
	sort.Strings(got)
	return got
}

func TestRefreshCompanyFacets(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertCompany(t, pool, "acme", "Acme Corp")
	insertCompany(t, pool, "plain", "Plain Co")

	// Acme: two open enriched jobs with overlapping facets + one closed job whose
	// facets must be excluded.
	insertJobWithFacets(t, pool, "acme:1", "acme",
		[]string{"europe"}, []string{"de"},
		`{"domains":["fintech"],"company_type":"startup","company_size":"11-50"}`)
	insertJobWithFacets(t, pool, "acme:2", "acme",
		[]string{"europe", "asia"}, []string{"de", "sg"},
		`{"domains":["fintech","ecommerce"],"company_type":"product","company_size":"11-50"}`)
	insertJobWithFacets(t, pool, "acme:closed", "acme",
		[]string{"africa"}, []string{"ng"},
		`{"domains":["gaming"],"company_type":"agency","company_size":"1000+"}`)
	closeJobByExtID(t, pool, "acme:closed")

	// Plain Co: one open, never-enriched job — geography present, enrichment empty.
	insertJobWithFacets(t, pool, "plain:1", "plain",
		[]string{"north_america"}, []string{"us"}, `{}`)

	t.Run("unions derived from open jobs only", func(t *testing.T) {
		if _, err := q.RefreshCompanyFacets(ctx); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if got := companyTextArray(t, pool, "acme", "regions"); !slices.Equal(got, []string{"asia", "europe"}) {
			t.Errorf("acme regions = %v, want [asia europe] (closed africa excluded)", got)
		}
		if got := companyTextArray(t, pool, "acme", "countries"); !slices.Equal(got, []string{"de", "sg"}) {
			t.Errorf("acme countries = %v, want [de sg] (closed ng excluded)", got)
		}
		if got := companyTextArray(t, pool, "acme", "domains"); !slices.Equal(got, []string{"ecommerce", "fintech"}) {
			t.Errorf("acme domains = %v, want [ecommerce fintech] (closed gaming excluded)", got)
		}
		if got := companyTextArray(t, pool, "acme", "company_types"); !slices.Equal(got, []string{"product", "startup"}) {
			t.Errorf("acme company_types = %v, want [product startup]", got)
		}
		if got := companyTextArray(t, pool, "acme", "company_sizes"); !slices.Equal(got, []string{"11-50"}) {
			t.Errorf("acme company_sizes = %v, want [11-50]", got)
		}
	})

	t.Run("unenriched job contributes no enrichment facets", func(t *testing.T) {
		if got := companyTextArray(t, pool, "plain", "regions"); !slices.Equal(got, []string{"north_america"}) {
			t.Errorf("plain regions = %v, want [north_america]", got)
		}
		if got := companyTextArray(t, pool, "plain", "domains"); len(got) != 0 {
			t.Errorf("plain domains = %v, want empty", got)
		}
		if got := companyTextArray(t, pool, "plain", "company_types"); len(got) != 0 {
			t.Errorf("plain company_types = %v, want empty", got)
		}
	})

	t.Run("re-running rewrites nothing", func(t *testing.T) {
		rows, err := q.RefreshCompanyFacets(ctx)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if rows != 0 {
			t.Errorf("idempotent refresh affected %d rows, want 0", rows)
		}
	})

	t.Run("closing all jobs empties the facet arrays", func(t *testing.T) {
		closeJobByExtID(t, pool, "acme:1")
		closeJobByExtID(t, pool, "acme:2")
		if _, err := q.RefreshCompanyFacets(ctx); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		for _, col := range []string{"regions", "countries", "domains", "company_types", "company_sizes"} {
			if got := companyTextArray(t, pool, "acme", col); len(got) != 0 {
				t.Errorf("acme %s = %v after all jobs closed, want empty", col, got)
			}
		}
	})
}
