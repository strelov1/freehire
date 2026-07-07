//go:build integration

// End-to-end HTTP verification of the job-reality signal on the detail endpoint:
// GET /api/v1/jobs/:slug drives the real path — GetJobBySlug → RoleClusterCount →
// jobview.ClassifyReality → JSON serialization — so the served `reality` object and
// its evidence are what a client actually receives.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

func seedRealityJob(t *testing.T, pool *pgxpool.Pool, q *db.Queries, extID, slug, desc string) {
	t.Helper()
	p := db.UpsertJobParams{
		Source: "greenhouse", ExternalID: extID, URL: "https://ex.test/" + extID,
		Title: "Staff Engineer", Company: "Acme", CompanySlug: "acme",
		PublicSlug: slug, Location: "Remote", Remote: true, Description: desc,
	}
	p.RoleFingerprint = pgtype.Text{String: "fp-" + extID, Valid: true}
	if _, err := q.UpsertJob(context.Background(), p); err != nil {
		t.Fatalf("seed %s: %v", extID, err)
	}
}

func TestGetJob_ServesRealitySignal(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	h := &API{pool: pool, queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug", h.GetJob)

	getReality := func(t *testing.T, slug string) map[string]any {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/jobs/"+slug, nil))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, body)
		}
		var out struct {
			Data struct {
				Reality map[string]any `json:"reality"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Data.Reality == nil {
			t.Fatal("response has no reality object")
		}
		return out.Data.Reality
	}

	t.Run("old + evergreen text converges to likely-evergreen", func(t *testing.T) {
		seedRealityJob(t, pool, q, "acme:ever", "ever-slug", "We are always hiring for this role.")
		if _, err := pool.Exec(ctx,
			"UPDATE jobs SET created_at = now() - interval '240 days', posted_at = now() - interval '240 days' WHERE public_slug = $1",
			"ever-slug"); err != nil {
			t.Fatalf("age job: %v", err)
		}
		r := getReality(t, "ever-slug")
		if r["class"] != "likely-evergreen" {
			t.Errorf("class = %v, want likely-evergreen", r["class"])
		}
		if age, _ := r["age_days"].(float64); age < 239 {
			t.Errorf("age_days = %v, want ~240", r["age_days"])
		}
	})

	t.Run("a brand-new plain posting is fresh", func(t *testing.T) {
		seedRealityJob(t, pool, q, "acme:new", "new-slug", "Own the checkout service.")
		r := getReality(t, "new-slug")
		if r["class"] != "fresh" {
			t.Errorf("class = %v, want fresh", r["class"])
		}
	})
}
