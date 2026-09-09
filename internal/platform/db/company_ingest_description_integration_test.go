//go:build integration

// Integration tests for FillCompanyDescriptionFromIngest: inserts a new company as
// a real (non-reference) row when the slug doesn't exist yet, fills a blank tagline
// and merges company_info without overwriting another source's values, and never
// touches job_count/collections for an existing job-backed company. Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFillCompanyDescriptionFromIngest(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	t.Run("a brand new slug is inserted as a real, non-reference row", func(t *testing.T) {
		err := q.FillCompanyDescriptionFromIngest(ctx, FillCompanyDescriptionFromIngestParams{
			Slug:        "coinbase",
			Name:        "Coinbase",
			Tagline:     pgtype.Text{},
			CompanyInfo: json.RawMessage(`{"summary":"Ready to be pushed beyond what you think you're capable of?"}`),
		})
		if err != nil {
			t.Fatalf("fill: %v", err)
		}

		c, err := q.GetCompany(ctx, "coinbase")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if c.IsReference {
			t.Error("is_reference = true, want false for an ingest-discovered company")
		}
		if c.JobCount != 0 {
			t.Errorf("job_count = %d, want 0 (this write does not touch it)", c.JobCount)
		}
		if companyInfoString(t, c.CompanyInfo, "summary") == "" {
			t.Error("company_info.summary missing")
		}
		if !c.CompanyInfoAt.Valid {
			t.Error("company_info_at not set")
		}
	})

	t.Run("existing job-backed company gets its tagline/company_info filled without disturbing jobs", func(t *testing.T) {
		insertCompany(t, pool, "figma", "Figma")
		insertJobWithFacets(t, pool, "figma:1", "figma", []string{}, []string{}, "{}")
		if _, err := pool.Exec(ctx, `UPDATE companies SET job_count = 9, collections = ARRAY['yc'] WHERE slug = 'figma'`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		err := q.FillCompanyDescriptionFromIngest(ctx, FillCompanyDescriptionFromIngestParams{
			Slug:        "figma",
			Name:        "Figma",
			Tagline:     pgtype.Text{},
			CompanyInfo: json.RawMessage(`{"summary":"Figma is growing our team of passionate creatives and builders."}`),
		})
		if err != nil {
			t.Fatalf("fill: %v", err)
		}

		c, err := q.GetCompany(ctx, "figma")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if companyInfoString(t, c.CompanyInfo, "summary") == "" {
			t.Error("company_info.summary missing")
		}
		if c.JobCount != 9 || len(c.Collections) != 1 || c.Collections[0] != "yc" || c.IsReference {
			t.Errorf("job_count/collections/is_reference disturbed: %d %v %v", c.JobCount, c.Collections, c.IsReference)
		}
	})

	t.Run("never overwrites a tagline or company_info key from another source", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, tagline, company_info)
			 VALUES ('acme', 'Acme', 'Tagline from YC', '{"website":"https://acme.example"}'::jsonb)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		err := q.FillCompanyDescriptionFromIngest(ctx, FillCompanyDescriptionFromIngestParams{
			Slug:        "acme",
			Name:        "Acme",
			Tagline:     pgtype.Text{String: "Tagline from ingest", Valid: true},
			CompanyInfo: json.RawMessage(`{"summary":"From ingest","website":"https://from-ingest.example"}`),
		})
		if err != nil {
			t.Fatalf("fill: %v", err)
		}

		c, err := q.GetCompany(ctx, "acme")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if c.Tagline.String != "Tagline from YC" {
			t.Errorf("tagline = %q, want the stored one preserved", c.Tagline.String)
		}
		if got := companyInfoString(t, c.CompanyInfo, "website"); got != "https://acme.example" {
			t.Errorf("company_info.website = %q, want the stored one preserved", got)
		}
		if got := companyInfoString(t, c.CompanyInfo, "summary"); got != "From ingest" {
			t.Errorf("company_info.summary = %q, want the new key merged in", got)
		}
	})
}
