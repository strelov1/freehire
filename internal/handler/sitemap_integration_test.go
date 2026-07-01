//go:build integration

// Integration test for the sitemap slice/boundary endpoints: they return the slim
// {slug, updated_at} wire shape, page by the keyset cursor, and — critically —
// resolve as static routes rather than being swallowed by the /jobs/:slug and
// /companies/:slug catch-alls. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

func seedSitemapJob(t *testing.T, q *db.Queries, n int) {
	t.Helper()
	_, err := q.UpsertJob(context.Background(), db.UpsertJobParams{
		Source:      "greenhouse",
		ExternalID:  fmt.Sprintf("acme:%02d", n),
		URL:         "https://example.test/job",
		Title:       fmt.Sprintf("Job %02d", n),
		Company:     fmt.Sprintf("Co %02d", n),
		CompanySlug: fmt.Sprintf("co-%02d", n),
		PublicSlug:  fmt.Sprintf("job-%02d", n),
		Location:    "Remote",
		Remote:      true,
		Description: "Build things.",
	})
	if err != nil {
		t.Fatalf("seed job %d: %v", n, err)
	}
}

func TestSitemapEndpoints(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	for i := 1; i <= 5; i++ {
		seedSitemapJob(t, q, i) // also upserts company co-0i
	}

	h := &API{pool: pool, queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// Sitemap routes are registered BEFORE the :slug catch-alls, mirroring wiring.
	app.Get("/api/v1/jobs/sitemap", h.JobSitemap)
	app.Get("/api/v1/companies/sitemap", h.CompanySitemap)
	app.Get("/api/v1/companies/sitemap/boundaries", h.CompanySitemapBoundaries)
	app.Get("/api/v1/jobs/:slug", h.GetJob)
	app.Get("/api/v1/companies/:slug", h.GetCompany)

	get := func(t *testing.T, url string) []byte {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d for %q, want 200 (body %s)", resp.StatusCode, url, body)
		}
		return body
	}

	type entry struct {
		Slug      string `json:"slug"`
		UpdatedAt string `json:"updated_at"`
	}
	decodeEntries := func(t *testing.T, body []byte) []entry {
		t.Helper()
		var d struct {
			Data []entry `json:"data"`
		}
		if err := json.Unmarshal(body, &d); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		return d.Data
	}

	t.Run("job sitemap is slim, freshest-first, and not captured by :slug", func(t *testing.T) {
		got := decodeEntries(t, get(t, "/api/v1/jobs/sitemap"))
		// Seeded job-01..job-05 with ascending ids -> newest id first.
		if len(got) != 5 || got[0].Slug != "job-05" || got[4].Slug != "job-01" {
			t.Fatalf("page = %+v, want job-05..job-01", got)
		}
		if got[0].UpdatedAt == "" {
			t.Fatalf("entry missing updated_at: %+v", got[0])
		}
	})

	t.Run("company slice pages by slug cursor", func(t *testing.T) {
		got := decodeEntries(t, get(t, "/api/v1/companies/sitemap?after=co-02&limit=2"))
		if len(got) != 2 || got[0].Slug != "co-03" || got[1].Slug != "co-04" {
			t.Fatalf("page = %+v, want [co-03 co-04]", got)
		}
	})

	t.Run("company boundaries", func(t *testing.T) {
		var d struct {
			Data []string `json:"data"`
		}
		if err := json.Unmarshal(get(t, "/api/v1/companies/sitemap/boundaries?chunk=2"), &d); err != nil {
			t.Fatalf("decode boundaries: %v", err)
		}
		want := []string{"co-02", "co-04"}
		if fmt.Sprint(d.Data) != fmt.Sprint(want) {
			t.Fatalf("boundaries = %v, want %v", d.Data, want)
		}
	})
}
