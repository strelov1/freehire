//go:build integration

// Integration tests for the Wikipedia company-info backfill's SQL layer:
// ListCompaniesMissingWikipediaInfo excludes companies that already have a
// tagline or were already checked; FillCompanyInfoFromWikipedia fills a blank
// tagline and merges company_info without overwriting another source's values;
// MarkCompanyWikipediaChecked touches only the checkpoint column. Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestListCompaniesMissingWikipediaInfo(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	insertCompany(t, pool, "has-tagline", "Has Tagline")
	if _, err := pool.Exec(ctx, `UPDATE companies SET tagline = 'Already set' WHERE slug = 'has-tagline'`); err != nil {
		t.Fatalf("seed tagline: %v", err)
	}
	insertCompany(t, pool, "already-checked", "Already Checked")
	if _, err := pool.Exec(ctx, `UPDATE companies SET company_info_wikipedia_checked_at = now() WHERE slug = 'already-checked'`); err != nil {
		t.Fatalf("seed checked_at: %v", err)
	}
	insertCompany(t, pool, "eligible", "Eligible Co")

	rows, err := q.ListCompaniesMissingWikipediaInfo(ctx, ListCompaniesMissingWikipediaInfoParams{
		AfterSlug: "",
		RowLimit:  100,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	var slugs []string
	for _, r := range rows {
		slugs = append(slugs, r.Slug)
	}
	if len(slugs) != 1 || slugs[0] != "eligible" {
		t.Errorf("candidates = %v, want only [eligible]", slugs)
	}
}

func TestFillCompanyInfoFromWikipedia(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	t.Run("fills a blank tagline and merges company_info", func(t *testing.T) {
		insertCompany(t, pool, "paladin-energy", "Paladin Energy")

		err := q.FillCompanyInfoFromWikipedia(ctx, FillCompanyInfoFromWikipediaParams{
			Slug:        "paladin-energy",
			Tagline:     pgtype.Text{String: "Uranium company based in Western Australia", Valid: true},
			CompanyInfo: json.RawMessage(`{"summary":"Paladin Energy Ltd is a uranium producer."}`),
		})
		if err != nil {
			t.Fatalf("fill: %v", err)
		}

		c, err := q.GetCompany(ctx, "paladin-energy")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if c.Tagline.String != "Uranium company based in Western Australia" {
			t.Errorf("tagline = %q, want the Wikipedia match", c.Tagline.String)
		}
		if companyInfoString(t, c.CompanyInfo, "summary") != "Paladin Energy Ltd is a uranium producer." {
			t.Errorf("company_info.summary = %s, want the Wikipedia extract", c.CompanyInfo)
		}
		if !c.CompanyInfoWikipediaCheckedAt.Valid {
			t.Error("company_info_wikipedia_checked_at not set")
		}
		if !c.CompanyInfoAt.Valid {
			t.Error("company_info_at not set")
		}
	})

	t.Run("never overwrites a tagline or company_info key from another source", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, tagline, company_info)
			 VALUES ('acme', 'Acme', 'Tagline from YC', '{"website":"https://acme.example"}'::jsonb)`); err != nil {
			t.Fatalf("seed: %v", err)
		}

		err := q.FillCompanyInfoFromWikipedia(ctx, FillCompanyInfoFromWikipediaParams{
			Slug:        "acme",
			Tagline:     pgtype.Text{String: "Tagline from Wikipedia", Valid: true},
			CompanyInfo: json.RawMessage(`{"summary":"From Wikipedia","website":"https://from-wikipedia.example"}`),
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
		if got := companyInfoString(t, c.CompanyInfo, "summary"); got != "From Wikipedia" {
			t.Errorf("company_info.summary = %q, want the new key merged in", got)
		}
	})
}

func TestMarkCompanyWikipediaChecked(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertCompany(t, pool, "no-match", "No Match Co")

	if err := q.MarkCompanyWikipediaChecked(ctx, "no-match"); err != nil {
		t.Fatalf("mark checked: %v", err)
	}

	c, err := q.GetCompany(ctx, "no-match")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !c.CompanyInfoWikipediaCheckedAt.Valid {
		t.Error("company_info_wikipedia_checked_at not set")
	}
	if c.Tagline.Valid && c.Tagline.String != "" {
		t.Errorf("tagline = %q, want untouched", c.Tagline.String)
	}
	if c.CompanyInfoAt.Valid {
		t.Error("company_info_at should stay unset when no match was found")
	}

	rows, err := q.ListCompaniesMissingWikipediaInfo(ctx, ListCompaniesMissingWikipediaInfoParams{RowLimit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.Slug == "no-match" {
			t.Error("a checked-but-unmatched company must not be listed again")
		}
	}
}
