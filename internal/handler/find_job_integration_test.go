//go:build integration

// Integration tests for GET /api/v1/jobs/find, the endpoint the browser extension asks
// "is the page I am on a posting you carry?". Both tiers need a real Postgres: the
// identity lookup by (source, external_id), and the fall-through that compares the page
// URL against jobs.url through normalize_job_url.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// findSlug calls the endpoint and returns the resolved slug, or "" for {"data": null}.
func findSlug(t *testing.T, app *fiber.App, url string) string {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/find?url="+url, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET find?url=%s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET find?url=%s: status %d, want 200", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out struct {
		Data *struct {
			PublicSlug string `json:"public_slug"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if out.Data == nil {
		return ""
	}
	return out.Data.PublicSlug
}

func TestFindJobResolvesByIdentityAndByURL(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	// A Greenhouse posting, whose identity the URL parser can recover, and an aggregator
	// posting, whose identity it cannot — the latter is only reachable by its stored URL.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug) VALUES
		 ('greenhouse', 'vgw-eu:8617539002', 'https://job-boards.greenhouse.io/vgw-eu/jobs/8617539002', 'Engineer', 'engineer-vgw-eu'),
		 ('himalayas', ':https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer',
		  'https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer', 'Staff Java Backend Developer', 'staff-java-mindera')`); err != nil {
		t.Fatalf("seed jobs: %v", err)
	}

	h := &jobsHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/find", h.FindJob)

	t.Run("greenhouse URL resolves by identity", func(t *testing.T) {
		if slug := findSlug(t, app, "https%3A%2F%2Fjob-boards.greenhouse.io%2Fvgw-eu%2Fjobs%2F8617539002"); slug != "engineer-vgw-eu" {
			t.Errorf("slug = %q, want engineer-vgw-eu", slug)
		}
	})

	t.Run("aggregator page resolves by its stored URL", func(t *testing.T) {
		// Carrying the tracking tag freehire stamps on its own outbound links.
		const page = "https%3A%2F%2Fhimalayas.app%2Fcompanies%2Fmindera%2Fjobs%2Fstaff-java-backend-developer%3Futm_source%3Dfreehire.me"
		if slug := findSlug(t, app, page); slug != "staff-java-mindera" {
			t.Errorf("slug = %q, want staff-java-mindera", slug)
		}
	})

	t.Run("unknown page is unresolved", func(t *testing.T) {
		if slug := findSlug(t, app, "https%3A%2F%2Fexample.test%2Fcareers%2Fsome-job"); slug != "" {
			t.Errorf("slug = %q, want no match", slug)
		}
	})

	t.Run("recognised identity with no posting falls through to the URL lookup", func(t *testing.T) {
		if slug := findSlug(t, app, "https%3A%2F%2Fjob-boards.greenhouse.io%2Fnobody%2Fjobs%2F1"); slug != "" {
			t.Errorf("slug = %q, want no match", slug)
		}
	})

	t.Run("an empty url matches nothing", func(t *testing.T) {
		// A posting can carry an empty url (nothing constrains the column), and an empty
		// query would otherwise normalize to the same "" and resolve to it.
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', 'blank-url', '', 'Nameless', 'nameless-job')`); err != nil {
			t.Fatalf("seed blank-url job: %v", err)
		}
		if slug := findSlug(t, app, ""); slug != "" {
			t.Errorf("slug = %q, want no match", slug)
		}
	})
}
