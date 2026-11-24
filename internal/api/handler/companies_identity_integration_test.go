//go:build integration

// Integration test for what lets a caller outside freehire use a /companies row: the
// company's site and LinkedIn page on every row, and the hq_country filter (where the
// company is headquartered, as opposed to `countries`, where its open jobs are). Only a
// real Postgres exercises the company_info projection and the case-folded comparison.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

type companyIdentityRow struct {
	Slug     string  `json:"slug"`
	Website  *string `json:"website"`
	Linkedin *string `json:"linkedin"`
}

func TestListCompaniesIdentityAndHQ(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	seed := func(slug, hq string, countries []string, info string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, job_count, hq_country, countries, company_info)
			 VALUES ($1, $1, 1, NULLIF($2, ''), $3, $4::jsonb)`,
			slug, hq, countries, info); err != nil {
			t.Fatalf("seed %q: %v", slug, err)
		}
	}
	// Stored codes are mixed case in production ("US" and "us" both occur).
	seed("canva", "AU", []string{"au", "us"}, `{"website":" https://canva.com ","linkedin":"https://www.linkedin.com/company/canva"}`)
	seed("atlassian", "au", []string{"au"}, `{"homepage":"atlassian.com"}`)
	seed("dollar-tree", "us", []string{"au", "us"}, `{}`)
	seed("unknown-hq", "", []string{"au"}, `{"website":"  "}`)

	h := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountCompaniesContract(app, h)

	list := func(t *testing.T, url string) map[string]companyIdentityRow {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil), -1)
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s: status = %d, want 200", url, resp.StatusCode)
		}
		var body struct {
			Data []companyIdentityRow `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out := make(map[string]companyIdentityRow, len(body.Data))
		for _, r := range body.Data {
			out[r.Slug] = r
		}
		return out
	}
	slugsOf := func(rows map[string]companyIdentityRow) []string {
		out := make([]string, 0, len(rows))
		for s := range rows {
			out = append(out, s)
		}
		slices.Sort(out)
		return out
	}

	t.Run("rows carry the site and LinkedIn page", func(t *testing.T) {
		rows := list(t, "/api/v1/companies")
		str := func(p *string) string {
			if p == nil {
				return "<null>"
			}
			return *p
		}
		for slug, want := range map[string][2]string{
			"canva":       {"https://canva.com", "https://www.linkedin.com/company/canva"},
			"atlassian":   {"atlassian.com", "<null>"},
			"dollar-tree": {"<null>", "<null>"},
			"unknown-hq":  {"<null>", "<null>"},
		} {
			got := rows[slug]
			if str(got.Website) != want[0] || str(got.Linkedin) != want[1] {
				t.Errorf("%s: website=%s linkedin=%s, want %s %s", slug, str(got.Website), str(got.Linkedin), want[0], want[1])
			}
		}
	})

	t.Run("hq_country matches the headquarters in any case", func(t *testing.T) {
		for _, url := range []string{"/api/v1/companies?hq_country=AU", "/api/v1/companies?hq_country=au"} {
			if got := slugsOf(list(t, url)); !slices.Equal(got, []string{"atlassian", "canva"}) {
				t.Errorf("%s → %v, want [atlassian canva]", url, got)
			}
		}
	})

	t.Run("countries still matches where the jobs are", func(t *testing.T) {
		got := slugsOf(list(t, "/api/v1/companies?countries=au"))
		if !slices.Equal(got, []string{"atlassian", "canva", "dollar-tree", "unknown-hq"}) {
			t.Errorf("countries=au → %v", got)
		}
	})
}
