//go:build integration

// Integration test for the facts GET /jobs/{slug} lifts to the top level: the employer's
// site and logo from its company record, and the employment type out of enrichment. Only a
// real Postgres exercises the company_info read. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestGetJobNamesTheEmployerSite(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, company_info) VALUES
		('motive', 'Motive', '{"website":" https://gomotive.com ","homepage":"wearemotive.com"}'),
		('nosite', 'No Site', '{}')`); err != nil {
		t.Fatal(err)
	}
	for slug, company := range map[string]string{"job-motive": "motive", "job-nosite": "nosite"} {
		id := seedJobSlug(t, pool, slug)
		if _, err := pool.Exec(ctx,
			`UPDATE jobs SET company_slug = $2, company = $3, employment_type = 'full_time' WHERE id = $1`,
			id, company, company); err != nil {
			t.Fatal(err)
		}
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountJobsContract(app, &jobsHandlers{queries: db.New(pool)}, middleware{})
	get := func(slug string) map[string]any {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/jobs/"+slug, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("GET %s: %d %v", slug, resp.StatusCode, err)
		}
		return body.Data
	}

	got := get("job-motive")
	if got["company_website"] != "https://gomotive.com" {
		t.Errorf("company_website = %v, want the stated website over the homepage", got["company_website"])
	}
	if got["company_logo"] != "https://logo.freehire.me/motive?domain=gomotive.com" {
		t.Errorf("company_logo = %v", got["company_logo"])
	}
	if got["employment_type"] != "full_time" {
		t.Errorf("employment_type = %v, want full_time at the top level", got["employment_type"])
	}

	got = get("job-nosite")
	for _, key := range []string{"company_website", "company_logo"} {
		if v, ok := got[key]; ok {
			t.Errorf("%s = %v for a company without a site, want absent", key, v)
		}
	}
}
