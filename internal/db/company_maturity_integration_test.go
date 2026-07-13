//go:build integration

// Integration tests for the deterministic company `maturity` facet: RefreshCompanyFacets
// derives a single-valued lifecycle stage — government | startup | scaleup | enterprise
// | NULL(unknown) — from signals already on the company row (organization_type,
// yc_status, employee_count, year_founded) plus whether its open jobs come from an
// exclusively-government source (usajobs/neogov). Pure SQL CASE in the same set-based
// recompute, so it runs against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setCompanySignals sets the maturity input columns on an already-inserted company.
func setCompanySignals(t *testing.T, pool *pgxpool.Pool, slug, orgType string, employeeCount, yearFounded int, ycStatus []string) {
	t.Helper()
	var orgPtr *string
	if orgType != "" {
		orgPtr = &orgType
	}
	var empPtr, yearPtr *int
	if employeeCount > 0 {
		empPtr = &employeeCount
	}
	if yearFounded > 0 {
		yearPtr = &yearFounded
	}
	if ycStatus == nil {
		ycStatus = []string{} // yc_status is NOT NULL (defaults to '{}')
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE companies SET organization_type = $2, employee_count = $3, year_founded = $4, yc_status = $5 WHERE slug = $1`,
		slug, orgPtr, empPtr, yearPtr, ycStatus); err != nil {
		t.Fatalf("set signals %q: %v", slug, err)
	}
}

// insertJobFromSource seeds one open job for a company under a specific source (the
// gov-source signal reads jobs.source).
func insertJobFromSource(t *testing.T, pool *pgxpool.Pool, source, externalID, companySlug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		 VALUES ($1, $2, 'http://example.test', 'A job', 'job-' || $2, $3)`,
		source, externalID, companySlug); err != nil {
		t.Fatalf("insert job %q: %v", externalID, err)
	}
}

// companyMaturity reads the nullable scalar maturity off the company row.
func companyMaturity(t *testing.T, pool *pgxpool.Pool, slug string) *string {
	t.Helper()
	var got *string
	if err := pool.QueryRow(context.Background(),
		`SELECT maturity FROM companies WHERE slug = $1`, slug).Scan(&got); err != nil {
		t.Fatalf("read maturity %q: %v", slug, err)
	}
	return got
}

func TestRefreshCompanyMaturity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// YC small company → startup.
	insertCompany(t, pool, "yc", "YC Co")
	setCompanySignals(t, pool, "yc", "", 20, 0, []string{"Active"})
	// Young + small (no YC) → startup.
	insertCompany(t, pool, "young", "Young Co")
	setCompanySignals(t, pool, "young", "", 30, 2024, nil)
	// Government by organization_type → government.
	insertCompany(t, pool, "govorg", "Gov Org")
	setCompanySignals(t, pool, "govorg", "Government", 0, 0, nil)
	// Government by source (usajobs) — even with a big headcount, government wins.
	insertCompany(t, pool, "govsrc", "Gov Src")
	setCompanySignals(t, pool, "govsrc", "", 8000, 1900, nil)
	insertJobFromSource(t, pool, "usajobs", "gov:1", "govsrc")
	// Large headcount → enterprise.
	insertCompany(t, pool, "bigco", "Big Co")
	setCompanySignals(t, pool, "bigco", "", 5000, 1980, nil)
	// YC alumnus gone public and huge → enterprise, NOT startup (the YC badge is
	// only a startup signal while status is 'Active').
	insertCompany(t, pool, "ycgiant", "YC Giant")
	setCompanySignals(t, pool, "ycgiant", "", 6000, 2008, []string{"Public"})
	// Mid headcount → scaleup.
	insertCompany(t, pool, "midco", "Mid Co")
	setCompanySignals(t, pool, "midco", "", 200, 2010, nil)
	// No signal → NULL (unknown).
	insertCompany(t, pool, "unknown", "Unknown Co")

	if _, err := q.RefreshCompanyFacets(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	cases := []struct {
		slug string
		want *string
	}{
		{"yc", strptr("startup")},
		{"young", strptr("startup")},
		{"govorg", strptr("government")},
		{"govsrc", strptr("government")}, // government beats enterprise
		{"bigco", strptr("enterprise")},
		{"ycgiant", strptr("enterprise")}, // Public YC alumnus at scale → enterprise, not startup
		{"midco", strptr("scaleup")},
		{"unknown", nil},
	}
	for _, c := range cases {
		got := companyMaturity(t, pool, c.slug)
		if !eqStrPtr(got, c.want) {
			t.Errorf("%s maturity = %s, want %s", c.slug, showStrPtr(got), showStrPtr(c.want))
		}
	}
}

// setMaturity sets the scalar maturity column directly (bypassing the recompute) so
// the filter test controls the exact value, including NULL.
func setMaturity(t *testing.T, pool *pgxpool.Pool, slug string, maturity *string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE companies SET maturity = $2 WHERE slug = $1`, slug, maturity); err != nil {
		t.Fatalf("set maturity %q: %v", slug, err)
	}
}

func maturityRowSlugs(rows []ListCompaniesRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Slug
	}
	return out
}

func TestListCompaniesFilterByMaturity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertCompany(t, pool, "sc", "Startup Co")
	insertCompany(t, pool, "bc", "Big Co")
	insertCompany(t, pool, "uc", "Unknown Co")
	setMaturity(t, pool, "sc", strptr("startup"))
	setMaturity(t, pool, "bc", strptr("enterprise"))
	setMaturity(t, pool, "uc", nil)

	t.Run("single value filters and excludes NULL", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Maturity: []string{"startup"}, Limit: 50})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got := maturityRowSlugs(rows); len(got) != 1 || got[0] != "sc" {
			t.Errorf("maturity=startup → %v, want [sc]", got)
		}
		n, err := q.CountCompanies(ctx, CountCompaniesParams{Maturity: []string{"startup"}})
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("count maturity=startup = %d, want 1", n)
		}
	})

	t.Run("multiple values are OR-ed", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Maturity: []string{"startup", "enterprise"}, Limit: 50})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got := maturityRowSlugs(rows); len(got) != 2 {
			t.Errorf("maturity=startup,enterprise → %v, want 2 rows (sc,bc), excluding NULL uc", got)
		}
	})

	t.Run("empty filter is no constraint", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Limit: 50})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got := maturityRowSlugs(rows); len(got) != 3 {
			t.Errorf("no maturity filter → %v, want all 3", got)
		}
	})
}

func strptr(s string) *string { return &s }

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func showStrPtr(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}
