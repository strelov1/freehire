//go:build integration

// Integration tests for the YC directory upsert: UpsertYCCompany enriches an
// existing company (company-info columns + curated yc_batch/yc_status) without
// disturbing job_count/collections/job-derived facets, inserts an unmatched slug as
// a reference row, and is idempotent; and RefreshCompanyFacets leaves yc_batch/
// yc_status untouched. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// companyInfoString reads a string value by key out of the stored company_info
// JSONB (Postgres re-serializes JSONB, so parse rather than string-compare).
func companyInfoString(t *testing.T, raw []byte, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("company_info not valid JSON (%s): %v", raw, err)
	}
	s, _ := m[key].(string)
	return s
}

func ycParams(slug, name, batch, status string) UpsertYCCompanyParams {
	return UpsertYCCompanyParams{
		Slug:          slug,
		Name:          name,
		Industries:    []string{"Fintech", "SaaS"},
		YearFounded:   pgtype.Int4{Int32: 2011, Valid: true},
		EmployeeCount: pgtype.Int4{Int32: 58, Valid: true},
		HqCountry:     pgtype.Text{String: "us", Valid: true},
		Tagline:       pgtype.Text{String: "On-demand electronics", Valid: true},
		CompanyInfo:   json.RawMessage(`{"description":"Long text","website":"https://x.co"}`),
		YcBatch:       []string{batch},
		YcStatus:      []string{status},
		YcStage:       []string{"Growth"},
		YcFlags:       []string{"top_company"},
	}
}

func TestUpsertYCCompany(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	t.Run("existing company enriched without disturbing jobs/collections", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, job_count, collections, regions)
			 VALUES ('stripe', 'Stripe', 9, ARRAY['yc'], ARRAY['north_america'])`); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := q.UpsertYCCompany(ctx, ycParams("stripe", "Stripe Renamed", "Summer 2009", "Public")); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		c, err := q.GetCompany(ctx, "stripe")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if len(c.YcBatch) != 1 || c.YcBatch[0] != "Summer 2009" || len(c.YcStatus) != 1 || c.YcStatus[0] != "Public" {
			t.Errorf("yc facets = %v/%v", c.YcBatch, c.YcStatus)
		}
		if len(c.YcStage) != 1 || c.YcStage[0] != "Growth" || len(c.YcFlags) != 1 || c.YcFlags[0] != "top_company" {
			t.Errorf("yc_stage/yc_flags = %v/%v", c.YcStage, c.YcFlags)
		}
		if c.EmployeeCount.Int32 != 58 || c.YearFounded.Int32 != 2011 || c.HqCountry.String != "us" {
			t.Errorf("company-info not applied: %+v", c)
		}
		if companyInfoString(t, c.CompanyInfo, "description") != "Long text" {
			t.Errorf("description not stored: %s", c.CompanyInfo)
		}
		// Untouched.
		if c.Name != "Stripe" || c.JobCount != 9 || c.IsReference {
			t.Errorf("name/job_count/is_reference disturbed: %q %d %v", c.Name, c.JobCount, c.IsReference)
		}
		if len(c.Collections) != 1 || c.Collections[0] != "yc" || len(c.Regions) != 1 || c.Regions[0] != "north_america" {
			t.Errorf("collections/regions disturbed: %v %v", c.Collections, c.Regions)
		}
	})

	t.Run("unmatched slug is inserted as a reference row", func(t *testing.T) {
		if err := q.UpsertYCCompany(ctx, ycParams("newyc", "New YC Co", "Winter 2024", "Active")); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		c, err := q.GetCompany(ctx, "newyc")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !c.IsReference {
			t.Error("is_reference = false, want true for inserted reference row")
		}
		if c.JobCount != 0 || len(c.YcStatus) != 1 || c.YcStatus[0] != "Active" {
			t.Errorf("reference row wrong: job_count=%d yc_status=%v", c.JobCount, c.YcStatus)
		}
	})

	t.Run("re-running is idempotent", func(t *testing.T) {
		if err := q.UpsertYCCompany(ctx, ycParams("newyc", "New YC Co", "Winter 2024", "Active")); err != nil {
			t.Fatalf("upsert 2: %v", err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM companies WHERE slug='newyc'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("newyc row count = %d, want 1", n)
		}
	})
}

func TestRefreshCompanyFacetsLeavesYCFacets(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	insertCompany(t, pool, "stripe", "Stripe")
	insertJobWithMode(t, pool, "stripe:1", "stripe", "remote", []string{"eu"})
	if err := q.UpsertYCCompany(ctx, ycParams("stripe", "Stripe", "Summer 2009", "Public")); err != nil {
		t.Fatalf("upsert yc: %v", err)
	}

	if _, err := q.RefreshCompanyFacets(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	c, err := q.GetCompany(ctx, "stripe")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Recompute populated remote_regions but left the curated yc facets alone.
	if len(c.RemoteRegions) != 1 || c.RemoteRegions[0] != "eu" {
		t.Errorf("remote_regions = %v, want [eu]", c.RemoteRegions)
	}
	if len(c.YcBatch) != 1 || c.YcBatch[0] != "Summer 2009" || len(c.YcStatus) != 1 || c.YcStatus[0] != "Public" ||
		len(c.YcStage) != 1 || c.YcStage[0] != "Growth" || len(c.YcFlags) != 1 || c.YcFlags[0] != "top_company" {
		t.Errorf("yc facets disturbed by recompute: %v/%v/%v/%v", c.YcBatch, c.YcStatus, c.YcStage, c.YcFlags)
	}
}

// The YC importer is no longer the only writer of tagline/company_info/industries,
// so it must fill gaps rather than replace. Before this test the upsert wrote all
// three by replacement, which erased whatever another source had stored on the
// importer's next scheduled run.
func TestUpsertYCCompanyPreservesValuesFromOtherSources(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, tagline, company_info, industries)
		 VALUES ('acme', 'Acme',
		         'Tagline written by another source',
		         '{"website":"https://acme.example","employee_range":"51-200"}'::jsonb,
		         ARRAY['logistics'])`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p := ycParams("acme", "Acme", "Winter 2020", "Active")
	p.Industries = []string{"fintech"}
	p.Tagline = pgtype.Text{String: "Tagline from the YC directory", Valid: true}
	p.CompanyInfo = json.RawMessage(`{"website":"https://from-yc.example","yc_url":"https://ycombinator.com/acme"}`)
	if err := q.UpsertYCCompany(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	c, err := q.GetCompany(ctx, "acme")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if c.Tagline.String != "Tagline written by another source" {
		t.Errorf("tagline = %q, want the stored one preserved", c.Tagline.String)
	}
	if got := companyInfoString(t, c.CompanyInfo, "website"); got != "https://acme.example" {
		t.Errorf("company_info.website = %q, want the stored one preserved", got)
	}
	if got := companyInfoString(t, c.CompanyInfo, "employee_range"); got != "51-200" {
		t.Errorf("company_info.employee_range = %q, want it left alone", got)
	}
	if got := companyInfoString(t, c.CompanyInfo, "yc_url"); got == "" {
		t.Error("company_info.yc_url missing: the YC keys should still be merged in")
	}
	if len(c.Industries) != 2 || c.Industries[0] != "fintech" || c.Industries[1] != "logistics" {
		t.Errorf("industries = %v, want the sorted union [fintech logistics]", c.Industries)
	}
}

// An empty stored tagline is absent, not a value worth protecting.
func TestUpsertYCCompanyFillsAnEmptyTagline(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, tagline) VALUES ('blank', 'Blank', '')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p := ycParams("blank", "Blank", "Winter 2020", "Active")
	p.Tagline = pgtype.Text{String: "From the directory", Valid: true}
	if err := q.UpsertYCCompany(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	c, err := q.GetCompany(ctx, "blank")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.Tagline.String != "From the directory" {
		t.Errorf("tagline = %q, want the directory value to fill the blank", c.Tagline.String)
	}
}
