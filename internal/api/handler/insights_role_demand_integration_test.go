//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestInsightsRoleDemand(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := db.New(pool)
	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), func(c *fiber.Ctx) error { return c.Next() }, newContract(contractHandlers{statsHandlers: h}))

	type point struct {
		Week     string `json:"week"`
		Category string `json:"category"`
		NewJobs  int    `json:"new_jobs"`
	}
	read := func() []point {
		t.Helper()
		resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/insights/role-demand", nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("role demand returned %d, want 200", resp.StatusCode)
		}
		var out struct {
			Data []point `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out.Data == nil {
			t.Fatal("missing data array")
		}
		return out.Data
	}
	if points := read(); len(points) != 0 {
		t.Fatalf("before first rollup got %v", points)
	}

	for _, job := range []struct {
		id, category, posted, created string
		closed, private, duplicate    bool
	}{
		{"backlog", "backend", "2026-08-30T23:59:59Z", "2026-11-18", false, false, false},
		{"first-week", "backend", "2026-08-31T00:00:00Z", "2026-11-18", true, false, false},
		{"fallback", "backend", "", "2026-11-16", false, false, false},
		{"duplicate", "backend", "2026-11-16", "2026-11-17", false, false, true},
		{"private", "backend", "2026-11-16", "2026-11-17", false, true, false},
		{"partial-week", "backend", "2026-11-23T00:00:00Z", "2026-11-23", false, false, false},
		{"second-category", "devops", "2026-11-22T23:59:59Z", "2026-11-23", false, false, false},
	} {
		seedInsightsHandlerJob(t, ctx, pool, q, job.id, job.category, "", []string{}, []string{}, 0)
		_, err := pool.Exec(ctx, `UPDATE jobs SET
			posted_at = NULLIF($1, '')::timestamptz,
			created_at = $2::text::timestamp AT TIME ZONE 'UTC',
			closed_at = CASE WHEN $3::boolean THEN '2026-09-02T00:00:00Z'::timestamptz END,
			is_private = $4,
			duplicate_of_role = CASE WHEN $5::boolean THEN
				(SELECT id FROM jobs WHERE external_id = 'secret-ext-fallback') END
			WHERE external_id = $6`, job.posted, job.created, job.closed, job.private, job.duplicate, "secret-ext-"+job.id)
		if err != nil {
			t.Fatalf("set dates and visibility for %s: %v", job.id, err)
		}
	}

	cutoff := pgtype.Date{Time: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Valid: true}
	rows, err := q.ComputeInsightsRoleDemand(ctx, db.ComputeInsightsRoleDemandParams{
		WeekStart:  cutoff,
		Categories: []string{"backend", "devops"},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := make([]db.CopyInsightsRoleDemandParams, len(rows))
	for i, row := range rows {
		snapshot[i] = db.CopyInsightsRoleDemandParams(row)
	}
	if _, err := q.CopyInsightsRoleDemand(ctx, snapshot); err != nil {
		t.Fatal(err)
	}

	points := read()
	if len(points) != 24 {
		t.Fatalf("got %d points, want 24", len(points))
	}
	want := map[point]bool{
		{Week: "2026-08-31", Category: "backend", NewJobs: 1}: true,
		{Week: "2026-11-16", Category: "backend", NewJobs: 1}: true,
		{Week: "2026-11-16", Category: "devops", NewJobs: 1}:  true,
	}
	for _, p := range points {
		if p.NewJobs != 0 {
			if !want[p] {
				t.Errorf("unexpected nonzero point %+v", p)
			}
			delete(want, p)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing points: %v", want)
	}
}
