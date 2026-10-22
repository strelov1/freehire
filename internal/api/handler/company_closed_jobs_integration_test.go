//go:build integration

// Integration test for GET /api/v1/companies/:slug/closed-jobs, the company page's Closed
// tab. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestCompanyClosedJobs(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	for i := range 3 {
		ext := fmt.Sprintf("acme-closed-%d", i)
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, posted_at, closed_at)
			 VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, 'acme', now() - make_interval(days => $2), now())`,
			ext, i); err != nil {
			t.Fatalf("seed job: %v", err)
		}
	}

	h := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies/:slug/closed-jobs", h.CompanyClosedJobs)

	get := func(t *testing.T, url string) (status int, slugs []string, hasMore bool) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, "GET", url, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			return resp.StatusCode, nil, false
		}
		var body struct {
			Data []struct {
				Slug     string  `json:"public_slug"`
				ClosedAt *string `json:"closed_at"`
			} `json:"data"`
			Meta struct {
				HasMore bool `json:"has_more"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, j := range body.Data {
			if j.ClosedAt == nil {
				t.Errorf("job %s carries no closed_at", j.Slug)
			}
			slugs = append(slugs, j.Slug)
		}
		return resp.StatusCode, slugs, body.Meta.HasMore
	}

	t.Run("a page short of the end says there is more", func(t *testing.T) {
		_, slugs, more := get(t, "/api/v1/companies/acme/closed-jobs?limit=2")
		if len(slugs) != 2 || !more {
			t.Fatalf("slugs=%v has_more=%v, want 2 rows and has_more", slugs, more)
		}
	})
	t.Run("the last page says there is no more", func(t *testing.T) {
		_, slugs, more := get(t, "/api/v1/companies/acme/closed-jobs?limit=2&offset=2")
		if len(slugs) != 1 || more {
			t.Fatalf("slugs=%v has_more=%v, want 1 row and no more", slugs, more)
		}
	})
	t.Run("an unknown company is a 404", func(t *testing.T) {
		if status, _, _ := get(t, "/api/v1/companies/nope/closed-jobs"); status != fiber.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
	})
}
