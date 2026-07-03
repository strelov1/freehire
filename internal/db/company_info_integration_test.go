//go:build integration

// Integration tests for the company-info backfill query semantics: UpsertCompanyInfo
// inserts an unmatched slug as a reference row and refreshes only the company-info
// columns of an existing one (leaving job_count and the job-derived facets untouched),
// is idempotent, and DeleteOrphanCompanies preserves reference rows while still
// sweeping jobless non-reference companies. This is SQL behavior, verified against a
// real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// companyInfoHomepage reads the homepage out of the stored company_info JSONB. It
// parses rather than string-compares because Postgres re-serializes JSONB (key order
// and spacing are not preserved).
func companyInfoHomepage(t *testing.T, raw []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("company_info not valid JSON (%s): %v", raw, err)
	}
	s, _ := m["homepage"].(string)
	return s
}

func sampleCompanyInfo(slug, name string) UpsertCompanyInfoParams {
	return UpsertCompanyInfoParams{
		Slug:             slug,
		Name:             name,
		Industries:       []string{"Software", "Fintech"},
		YearFounded:      pgtype.Int4{Int32: 1999, Valid: true},
		EmployeeCount:    pgtype.Int4{Int32: 500, Valid: true},
		HqCountry:        pgtype.Text{String: "US", Valid: true},
		OrganizationType: pgtype.Text{String: "Private", Valid: true},
		Tagline:          pgtype.Text{String: "We do things", Valid: true},
		CompanyInfo:      json.RawMessage(`{"homepage":"acme.com"}`),
	}
}

func TestUpsertCompanyInfo(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	t.Run("unmatched slug is inserted as a reference row", func(t *testing.T) {
		if err := q.UpsertCompanyInfo(ctx, sampleCompanyInfo("acme", "Acme Corp")); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		c, err := q.GetCompany(ctx, "acme")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !c.IsReference {
			t.Error("is_reference = false, want true for an inserted reference row")
		}
		if c.JobCount != 0 {
			t.Errorf("job_count = %d, want 0", c.JobCount)
		}
		if len(c.Industries) != 2 || c.YearFounded.Int32 != 1999 || c.EmployeeCount.Int32 != 500 {
			t.Errorf("company-info columns not set: %+v", c)
		}
		if c.HqCountry.String != "US" || c.OrganizationType.String != "Private" {
			t.Errorf("hq/org not set: %q %q", c.HqCountry.String, c.OrganizationType.String)
		}
		if got := companyInfoHomepage(t, c.CompanyInfo); got != "acme.com" {
			t.Errorf("company_info homepage = %q (raw %s)", got, c.CompanyInfo)
		}
		if !c.CompanyInfoAt.Valid {
			t.Error("company_info_at not stamped")
		}
	})

	t.Run("existing company is enriched without disturbing jobs or facets", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, job_count, company_types, company_sizes)
			 VALUES ('globex', 'Globex', 7, ARRAY['Public'], ARRAY['201-500'])`); err != nil {
			t.Fatalf("seed globex: %v", err)
		}
		p := sampleCompanyInfo("globex", "Globex Renamed")
		if err := q.UpsertCompanyInfo(ctx, p); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		c, err := q.GetCompany(ctx, "globex")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if c.IsReference {
			t.Error("is_reference flipped to true on an existing company")
		}
		if c.JobCount != 7 {
			t.Errorf("job_count = %d, want 7 (untouched)", c.JobCount)
		}
		if len(c.CompanyTypes) != 1 || c.CompanyTypes[0] != "Public" {
			t.Errorf("company_types clobbered: %v", c.CompanyTypes)
		}
		if len(c.CompanySizes) != 1 || c.CompanySizes[0] != "201-500" {
			t.Errorf("company_sizes clobbered: %v", c.CompanySizes)
		}
		if c.Name != "Globex" {
			t.Errorf("name = %q, want unchanged \"Globex\" (display name is not overwritten)", c.Name)
		}
		if c.EmployeeCount.Int32 != 500 {
			t.Errorf("company-info not applied: employee_count = %v", c.EmployeeCount)
		}
	})

	t.Run("re-running is idempotent", func(t *testing.T) {
		p := sampleCompanyInfo("acme", "Acme Corp")
		if err := q.UpsertCompanyInfo(ctx, p); err != nil {
			t.Fatalf("upsert 2: %v", err)
		}
		c, err := q.GetCompany(ctx, "acme")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if c.EmployeeCount.Int32 != 500 || companyInfoHomepage(t, c.CompanyInfo) != "acme.com" {
			t.Errorf("idempotent re-run changed values: %+v", c)
		}
		// still exactly one acme row
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM companies WHERE slug = 'acme'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("acme row count = %d, want 1", n)
		}
	})
}

func TestCompanyExistsAndOrphanCleanupPreservesReference(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate jobs: %v", err)
	}

	// A job-backed company, a jobless non-reference orphan, and a jobless reference row.
	insertCompany(t, pool, "withjob", "With Job")
	insertJobWithFacets(t, pool, "j1", "withjob", []string{}, []string{}, "{}")
	insertCompany(t, pool, "orphan", "Orphan Co") // is_reference defaults false
	if err := q.UpsertCompanyInfo(ctx, sampleCompanyInfo("refco", "Ref Co")); err != nil {
		t.Fatalf("upsert refco: %v", err)
	}

	t.Run("CompanyExists reflects presence", func(t *testing.T) {
		for slug, want := range map[string]bool{"withjob": true, "refco": true, "missing": false} {
			got, err := q.CompanyExists(ctx, slug)
			if err != nil {
				t.Fatalf("exists %s: %v", slug, err)
			}
			if got != want {
				t.Errorf("CompanyExists(%q) = %v, want %v", slug, got, want)
			}
		}
	})

	t.Run("orphan cleanup deletes jobless non-reference only", func(t *testing.T) {
		if _, err := q.DeleteOrphanCompanies(ctx); err != nil {
			t.Fatalf("delete orphans: %v", err)
		}
		for slug, wantExists := range map[string]bool{"withjob": true, "refco": true, "orphan": false} {
			got, err := q.CompanyExists(ctx, slug)
			if err != nil {
				t.Fatalf("exists %s: %v", slug, err)
			}
			if got != wantExists {
				t.Errorf("after cleanup CompanyExists(%q) = %v, want %v", slug, got, wantExists)
			}
		}
	})
}
