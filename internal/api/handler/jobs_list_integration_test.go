//go:build integration

// End-to-end HTTP test for GET /jobs against a real Postgres. The listing runs no filter
// at all, so what is worth pinning is that it SAYS so: a caller who sends a filter param
// here gets the unfiltered catalogue, and meta.ignored_params is the only thing that
// distinguishes that from a real answer. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestListJobsReportsIgnoredParams(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'gh:list-1', 'http://ats.test/list-1', 'Go Dev', 'go-dev-acme-ffff0001')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	queries := db.New(pool)
	h := &jobsHandlers{queries: queries, cache: cache.NewMemory(), estimator: queries}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs", h.ListJobs)

	get := func(target string) (int, map[string]any) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, target, nil))
		if err != nil {
			t.Fatalf("request %s: %v", target, err)
		}
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}

	t.Run("a filter param is reported, not silently dropped", func(t *testing.T) {
		// `countries` is real search vocabulary — which is exactly why the report cannot
		// be built from that vocabulary. This endpoint reads no filter at all, so the
		// page came back unfiltered and the caller has to be told.
		status, body := get("/api/v1/jobs?countries=de&limit=1")
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		meta, _ := body["meta"].(map[string]any)
		ignored, _ := meta["ignored_params"].([]any)
		if len(ignored) != 1 {
			t.Fatalf("meta.ignored_params = %v, want one entry", meta["ignored_params"])
		}
		first, _ := ignored[0].(map[string]any)
		if first["param"] != "countries" {
			t.Errorf("ignored_params[0] = %v, want countries", first)
		}
		// No suggestion: nothing this endpoint reads is a facet, so pointing at a
		// near-miss facet name would recommend a param that is equally ignored.
		if _, present := first["did_you_mean"]; present {
			t.Errorf("ignored_params[0] = %v, want no did_you_mean", first)
		}
		if data, _ := body["data"].([]any); len(data) != 1 {
			t.Errorf("data len = %d, want the page served anyway", len(data))
		}
	})

	t.Run("the pagination params are the whole vocabulary and are not accused", func(t *testing.T) {
		_, body := get("/api/v1/jobs?limit=1&offset=0")
		meta, _ := body["meta"].(map[string]any)
		if _, present := meta["ignored_params"]; present {
			t.Errorf("meta.ignored_params = %v, want the key absent", meta["ignored_params"])
		}
		if meta["limit"] != float64(1) || meta["offset"] != float64(0) {
			t.Errorf("meta = %v, want the pagination echo preserved", meta)
		}
	})
}
