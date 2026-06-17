//go:build integration

// End-to-end HTTP test for the similar-jobs endpoint against a real Postgres: the
// route resolves the public :slug to the internal id, queries the (faked) search
// backend, and shapes the public list envelope. The search backend is faked — slug
// resolution is the only part that needs a real DB; Meilisearch is exercised in the
// search package's own integration test. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

func TestSimilarJobsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	const knownSlug = "go-dev-acme-aaaa1111"
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'gh:1', 'http://ats.test/1', 'Go Dev', $1)`, knownSlug); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	fake := &fakeSearcher{similarHits: []search.JobDocument{
		{ID: 2, Job: jobview.Job{PublicSlug: "py-dev-beta-bbbb2222", Title: "Py Dev"}},
		{ID: 3, Job: jobview.Job{PublicSlug: "rust-dev-ceta-cccc3333", Title: "Rust Dev"}},
	}}
	h := &API{pool: pool, queries: db.New(pool), search: fake}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug/similar", h.SimilarJobs)

	get := func(target string) (*http.Response, map[string]any) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, target, nil))
		if err != nil {
			t.Fatalf("request %s: %v", target, err)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp, body
	}

	t.Run("known slug returns neighbours without internal id", func(t *testing.T) {
		resp, body := get("/api/v1/jobs/" + knownSlug + "/similar")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, _ := body["data"].([]any)
		if len(data) != 2 {
			t.Fatalf("data len = %d, want 2", len(data))
		}
		first, _ := data[0].(map[string]any)
		if first["public_slug"] != "py-dev-beta-bbbb2222" {
			t.Errorf("public_slug = %v", first["public_slug"])
		}
		if _, leaked := first["id"]; leaked {
			t.Errorf("internal id leaked: %v", first)
		}
	})

	t.Run("limit is parsed, defaulted, and clamped", func(t *testing.T) {
		if _, _ = get("/api/v1/jobs/" + knownSlug + "/similar?limit=3"); fake.similarLimit != 3 {
			t.Errorf("explicit limit = %d, want 3", fake.similarLimit)
		}
		if _, _ = get("/api/v1/jobs/" + knownSlug + "/similar"); fake.similarLimit != defaultSimilarLimit {
			t.Errorf("default limit = %d, want %d", fake.similarLimit, defaultSimilarLimit)
		}
		if _, _ = get("/api/v1/jobs/" + knownSlug + "/similar?limit=999"); fake.similarLimit != maxSimilarLimit {
			t.Errorf("clamped limit = %d, want %d", fake.similarLimit, maxSimilarLimit)
		}
	})

	t.Run("unknown slug is a 404 and never reaches the backend", func(t *testing.T) {
		fake.similarLimit = 0
		resp, _ := get("/api/v1/jobs/no-such-slug/similar")
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
		if fake.similarLimit != 0 {
			t.Errorf("backend was queried for an unknown slug (limit=%d)", fake.similarLimit)
		}
	})
}
