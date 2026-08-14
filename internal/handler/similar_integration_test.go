//go:build integration

// End-to-end HTTP test for the similar-jobs endpoint against a real Postgres: the
// route resolves the public :slug to the internal id, reads the precomputed
// jobs.similar_job_ids list (jobs.similar_job_ids/similar_computed_at, populated
// offline by cmd/similar-backfill — see
// openspec/changes/drop-hybrid-search-pgvector-similar/design.md Decision 5),
// fetches the referenced jobs, and shapes the public list envelope. No live search
// backend involved (the old Meili-backed jobs_semantic path this endpoint used
// before is removed from the read path by this test's own change) — Postgres is
// the only dependency. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

func TestSimilarJobsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	seedJob := func(externalID, slug, title string, closed bool) int64 {
		var id int64
		closedExpr := "NULL"
		if closed {
			closedExpr = "now()"
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, closed_at)
			 VALUES ('greenhouse', $1, $2, $3, $4, `+closedExpr+`)
			 RETURNING id`,
			externalID, "http://ats.test/"+externalID, title, slug).Scan(&id); err != nil {
			t.Fatalf("seed job %s: %v", externalID, err)
		}
		return id
	}

	const knownSlug = "go-dev-acme-aaaa1111"
	sourceID := seedJob("gh:source", knownSlug, "Go Dev", false)

	// Eight open neighbours, kept in a fixed, easily-asserted title order.
	openIDs := make([]int64, 8)
	for i := range openIDs {
		externalID := "gh:open-" + string(rune('a'+i))
		slug := "neighbour-open-" + string(rune('a'+i)) + "-bbbb000" + string(rune('1'+i))
		openIDs[i] = seedJob(externalID, slug, "Open Neighbour", false)
	}
	closedID := seedJob("gh:closed", "neighbour-closed-cccc9999", "Closed Neighbour", true)
	const danglingID int64 = 987654321 // never inserted: simulates a hard-deleted (cmd/prune) stale entry.

	// Interleave the closed and dangling ids among the open ones so a passing test
	// proves order preservation AND graceful dropping at once, not just one or the
	// other: the stored order is
	//   [open0, open1, closed, open2, dangling, open3, open4, open5, open6, open7]
	// and the only correct filtered-and-reordered output is the open ids in their
	// original relative order with the other two removed.
	similarJobIDs := []int64{
		openIDs[0], openIDs[1], closedID, openIDs[2], danglingID,
		openIDs[3], openIDs[4], openIDs[5], openIDs[6], openIDs[7],
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET similar_job_ids = $1::bigint[], similar_computed_at = now() WHERE id = $2`,
		similarJobIDs, sourceID); err != nil {
		t.Fatalf("seed similar_job_ids: %v", err)
	}

	// A second source job that was never backfilled (similar_job_ids IS NULL) —
	// the "no precomputed list yet" case must serve an empty list, not error.
	const neverBackfilledSlug = "never-backfilled-acme-eeee5555"
	seedJob("gh:never-backfilled", neverBackfilledSlug, "Never Backfilled", false)

	h := &searchHandlers{queries: db.New(pool)}

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

	slugsOf := func(data []any) []string {
		out := make([]string, len(data))
		for i, d := range data {
			m, _ := d.(map[string]any)
			out[i], _ = m["public_slug"].(string)
		}
		return out
	}

	// Fetch each open neighbour's slug up front so assertions read by identity, not
	// by re-deriving the seeding scheme's slug string.
	openSlugs := make([]string, len(openIDs))
	for i, id := range openIDs {
		if err := pool.QueryRow(ctx, `SELECT public_slug FROM jobs WHERE id = $1`, id).Scan(&openSlugs[i]); err != nil {
			t.Fatalf("look up slug for %d: %v", id, err)
		}
	}

	t.Run("default limit: closed and dangling neighbours dropped, order preserved", func(t *testing.T) {
		resp, body := get("/api/v1/jobs/" + knownSlug + "/similar")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, _ := body["data"].([]any)
		if len(data) != defaultSimilarLimit {
			t.Fatalf("data len = %d, want %d", len(data), defaultSimilarLimit)
		}
		got := slugsOf(data)
		want := openSlugs[:defaultSimilarLimit] // open0..open5, in original order.
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("position %d: slug = %q, want %q (full got=%v)", i, got[i], want[i], got)
			}
		}
		first, _ := data[0].(map[string]any)
		if _, leaked := first["id"]; leaked {
			t.Errorf("internal id leaked: %v", first)
		}
	})

	t.Run("explicit limit is honored", func(t *testing.T) {
		_, body := get("/api/v1/jobs/" + knownSlug + "/similar?limit=3")
		data, _ := body["data"].([]any)
		if len(data) != 3 {
			t.Fatalf("data len = %d, want 3", len(data))
		}
		got := slugsOf(data)
		want := openSlugs[:3]
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("position %d: slug = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("limit above max is clamped, not an error, even when fewer results exist", func(t *testing.T) {
		resp, body := get("/api/v1/jobs/" + knownSlug + "/similar?limit=999")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, _ := body["data"].([]any)
		// Only 8 open neighbours were ever stored; a clamp-to-20 request must not
		// error or pad — it returns every open neighbour the stored list has.
		if len(data) != len(openIDs) {
			t.Fatalf("data len = %d, want %d (all open neighbours)", len(data), len(openIDs))
		}
	})

	t.Run("never-backfilled job serves an empty list, not an error", func(t *testing.T) {
		resp, body := get("/api/v1/jobs/" + neverBackfilledSlug + "/similar")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		data, ok := body["data"].([]any)
		if !ok {
			t.Fatalf("data field missing or wrong type: %v", body)
		}
		if len(data) != 0 {
			t.Errorf("data len = %d, want 0", len(data))
		}
	})

	t.Run("unknown slug is a 404", func(t *testing.T) {
		resp, _ := get("/api/v1/jobs/no-such-slug/similar")
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}
