//go:build integration

// Integration test for the retired-company-slug redirect: GET /api/v1/companies/:slug must
// 301 to the canonical company when a merge retired the slug, 404 when nobody has heard of
// it, and serve 200 when a company exists under it — even if an alias row also names it.
//
// The handler holds a concrete *db.Queries, so the wire contract can only be exercised
// against a real Postgres. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

func TestGetCompanyRedirectsARetiredSlug(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, job_count) VALUES ('dollar-tree', 'Dollar Tree', 5)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		 VALUES ('dollartree', 'dollar-tree', 'dollartree', 'spelling'),
		        ('dollar-tree', 'dollar-tree-holdings', 'dollartree', 'spelling')`); err != nil {
		// The second row is deliberate: a slug that is BOTH a live company and an alias.
		t.Fatalf("seed aliases: %v", err)
	}

	h := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies/:slug", h.GetCompany)

	do := func(t *testing.T, url string) (status int, location string) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Location")
	}

	t.Run("a retired slug redirects to its canonical company", func(t *testing.T) {
		status, loc := do(t, "/api/v1/companies/dollartree")
		if status != fiber.StatusMovedPermanently {
			t.Fatalf("status = %d, want 301", status)
		}
		if loc != "/api/v1/companies/dollar-tree" {
			t.Errorf("Location = %q, want /api/v1/companies/dollar-tree", loc)
		}
	})

	t.Run("the redirect keeps the query string", func(t *testing.T) {
		// Paging is in the query, so dropping it would land a crawler following page 4 of a
		// retired slug on page 1 of the canonical one.
		status, loc := do(t, "/api/v1/companies/dollartree?limit=20&offset=40")
		if status != fiber.StatusMovedPermanently {
			t.Fatalf("status = %d, want 301", status)
		}
		if loc != "/api/v1/companies/dollar-tree?limit=20&offset=40" {
			t.Errorf("Location = %q, want the query string preserved", loc)
		}
	})

	t.Run("a live company wins over an alias row naming it", func(t *testing.T) {
		// The alias lookup runs only after the company read misses, so a company that came
		// back is never shadowed by the row that once retired it.
		status, _ := do(t, "/api/v1/companies/dollar-tree")
		if status != fiber.StatusOK {
			t.Errorf("status = %d, want 200", status)
		}
	})

	t.Run("an unknown slug still 404s", func(t *testing.T) {
		status, _ := do(t, "/api/v1/companies/not-a-real-company")
		if status != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", status)
		}
	})
}
