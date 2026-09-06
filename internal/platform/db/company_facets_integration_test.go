//go:build integration

// Integration tests for the denormalized company facet arrays: RefreshCompanyFacets
// aggregates each company's regions/countries (from the jobs geography columns) and
// domains/company_types/company_sizes (from the jobs.enrichment JSONB) as the
// distinct union over its OPEN jobs (closed_at IS NULL), in the same set-based pass
// that maintains job_count. This is SQL behavior (array_agg over unnest /
// jsonb_array_elements_text + the IS DISTINCT FROM guard), so it runs against a real
// Postgres. Run with: go test -tags=integration ./internal/platform/db/
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
// unenriched job. The body and category are fixed rather than parameterized: the
// recompute only aggregates postings the job search index holds, so a row missing
// either would contribute to nothing and these tests would assert against empties.
func insertJobWithFacets(t *testing.T, pool *pgxpool.Pool, externalID, companySlug string, regions, countries []string, enrichment string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, regions, countries, enrichment,
		                   description, category)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3, $4, $5,
		         'We are hiring.', 'backend')`,
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
		if _, err := q.RefreshCompanyFacets(ctx, RefreshCompanyFacetsParams{}); err != nil {
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
		rows, err := q.RefreshCompanyFacets(ctx, RefreshCompanyFacetsParams{})
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
		if _, err := q.RefreshCompanyFacets(ctx, RefreshCompanyFacetsParams{}); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		for _, col := range []string{"regions", "countries", "domains", "company_types", "company_sizes"} {
			if got := companyTextArray(t, pool, "acme", col); len(got) != 0 {
				t.Errorf("acme %s = %v after all jobs closed, want empty", col, got)
			}
		}
	})
}

// TestRefreshCompanyFacetsIndustriesDerived covers the #2088 remainder: the
// industries_derived column bakes both the #2082 precedence (a curated company is
// never matched through its domains) and the new domain-count threshold (a company
// above it is not matched either) in at recompute time, so both query backends can
// filter `industries` with a plain OR against this column. The mapping passed here is
// a small literal fixture, not internal/dict/industrytag — internal/platform must
// never import internal/dict (see AGENTS.md), even from a test.
func TestRefreshCompanyFacetsIndustriesDerived(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	mapping := RefreshCompanyFacetsParams{
		MappingDomains:    []string{"devtools", "fintech", "healthcare"},
		MappingIndustries: []string{"developer-tools", "fintech", "healthcare"},
	}

	insertCompany(t, pool, "curated", "Curated Co")
	if _, err := pool.Exec(ctx, `UPDATE companies SET industries = '{fintech}' WHERE slug = 'curated'`); err != nil {
		t.Fatalf("seed curated industries: %v", err)
	}
	insertJobWithFacets(t, pool, "curated:1", "curated",
		[]string{"europe"}, []string{"de"}, `{"domains":["fintech"]}`)

	insertCompany(t, pool, "focused", "Focused Co")
	insertJobWithFacets(t, pool, "focused:1", "focused",
		[]string{"europe"}, []string{"de"}, `{"domains":["devtools","fintech"]}`)

	insertCompany(t, pool, "wide", "Wide Co")
	insertJobWithFacets(t, pool, "wide:1", "wide",
		[]string{"europe"}, []string{"de"}, `{"domains":["devtools","fintech","healthcare"]}`)

	if _, err := q.RefreshCompanyFacets(ctx, mapping); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	t.Run("a curated company's industries_derived is empty regardless of its domains", func(t *testing.T) {
		if got := companyTextArray(t, pool, "curated", "industries_derived"); len(got) != 0 {
			t.Errorf("curated industries_derived = %v, want empty", got)
		}
	})

	t.Run("a company at or under the domain-count threshold gets its domains mapped", func(t *testing.T) {
		if got := companyTextArray(t, pool, "focused", "industries_derived"); !slices.Equal(got, []string{"developer-tools", "fintech"}) {
			t.Errorf("focused industries_derived = %v, want [developer-tools fintech]", got)
		}
	})

	t.Run("a company above the domain-count threshold gets no derived industries", func(t *testing.T) {
		if got := companyTextArray(t, pool, "wide", "industries_derived"); len(got) != 0 {
			t.Errorf("wide industries_derived = %v, want empty (3 domains > threshold)", got)
		}
	})

	t.Run("re-running rewrites nothing", func(t *testing.T) {
		rows, err := q.RefreshCompanyFacets(ctx, mapping)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if rows != 0 {
			t.Errorf("idempotent refresh affected %d rows, want 0", rows)
		}
	})
}

// TestSetCompanyIndustriesClearsStaleDerived guards the #2082 precedence invariant
// (a curated company is never matched through its domains) across the window between
// an importer curating a company and the next periodic RefreshCompanyFacets run.
// Without this, a company freshly curated by cmd/import-company-industries would
// keep answering through a stale industries_derived left over from before it was
// curated — reproducing #2082 for however long that window is.
func TestSetCompanyIndustriesClearsStaleDerived(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	insertCompany(t, pool, "newly-curated", "Newly Curated Co")
	// Seed industries_derived as a prior RefreshCompanyFacets run would have left it:
	// no curated industry yet, so its domains answered "gaming".
	if _, err := pool.Exec(ctx,
		`UPDATE companies SET industries_derived = '{gaming}' WHERE slug = 'newly-curated'`); err != nil {
		t.Fatalf("seed stale industries_derived: %v", err)
	}

	t.Run("curating a company immediately clears its stale derived industries", func(t *testing.T) {
		if _, err := q.SetCompanyIndustries(ctx, SetCompanyIndustriesParams{
			Slug: "newly-curated", Industries: []string{"logistics"},
		}); err != nil {
			t.Fatalf("SetCompanyIndustries: %v", err)
		}
		if got := companyTextArray(t, pool, "newly-curated", "industries_derived"); len(got) != 0 {
			t.Errorf("industries_derived = %v after curating, want empty (not the stale 'gaming')", got)
		}
	})

	t.Run("uncurating a company leaves its derived industries for the next recompute", func(t *testing.T) {
		// The reverse direction only widens reach (never a false match), so it is left
		// to the ordinary periodic recompute like every other job-derived facet.
		if _, err := pool.Exec(ctx,
			`UPDATE companies SET industries_derived = '{fintech}' WHERE slug = 'newly-curated'`); err != nil {
			t.Fatalf("reseed industries_derived: %v", err)
		}
		if _, err := q.SetCompanyIndustries(ctx, SetCompanyIndustriesParams{
			Slug: "newly-curated", Industries: []string{},
		}); err != nil {
			t.Fatalf("SetCompanyIndustries: %v", err)
		}
		if got := companyTextArray(t, pool, "newly-curated", "industries_derived"); !slices.Equal(got, []string{"fintech"}) {
			t.Errorf("industries_derived = %v after uncurating, want unchanged [fintech]", got)
		}
	})
}
