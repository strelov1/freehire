//go:build integration

// Integration test for GET /api/v1/insights/role-pulse, the per-category market pulse.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestInsightsRolePulseEndpoint(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Three open backend postings, one of them without a seniority: the open count
	// must cover all three (the seniority-resolved rollup would say two).
	seedInsightsHandlerJob(t, ctx, pool, q, "1", "backend", "senior", []string{"de"}, []string{}, 0)
	seedInsightsHandlerJob(t, ctx, pool, q, "2", "backend", "junior", []string{"de"}, []string{}, 0)
	seedInsightsHandlerJob(t, ctx, pool, q, "3", "backend", "", []string{}, []string{}, 0)
	// One closed backend posting that counts as a close.
	seedInsightsHandlerJob(t, ctx, pool, q, "4", "backend", "", []string{}, []string{}, 0)
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET created_at = now() - interval '12 days', closed_at = now() - interval '1 day',
			closed_reason = 'unseen'
		WHERE external_id = 'secret-ext-4'`); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO job_daily_views (day, job_id, uniques, page_uniques)
		SELECT current_date - 1, id, 90, 6 FROM jobs WHERE external_id = 'secret-ext-1'`); err != nil {
		t.Fatalf("seed views: %v", err)
	}

	now := time.Now().UTC()
	prev := pgtype.Timestamptz{Time: now.AddDate(0, 0, -30), Valid: true}
	if err := q.DeleteAllInsightsRoleCountryStats(ctx); err != nil {
		t.Fatalf("delete role-country: %v", err)
	}
	if _, err := q.RebuildInsightsRoleCountryStatsGlobal(ctx, prev); err != nil {
		t.Fatalf("rebuild role-country: %v", err)
	}
	if err := q.DeleteAllInsightsRolePulse(ctx); err != nil {
		t.Fatalf("delete pulse: %v", err)
	}
	if _, err := q.RebuildInsightsRolePulse(ctx, db.RebuildInsightsRolePulseParams{
		WindowStart: prev,
		PriorStart:  pgtype.Timestamptz{Time: now.AddDate(0, 0, -60), Valid: true},
	}); err != nil {
		t.Fatalf("rebuild pulse: %v", err)
	}

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/role-pulse", h.InsightsRolePulse)

	t.Run("known category", func(t *testing.T) {
		out, raw, code := getInsights(t, ctx, app, "/api/v1/insights/role-pulse?category=backend")
		if code != fiber.StatusOK {
			t.Fatalf("status %d, want 200: %s", code, raw)
		}
		data, _ := out["data"].(map[string]any)
		if data["category"] != "backend" || data["open_count"] != float64(3) {
			t.Errorf("category/open_count = %v / %v, want backend / 3", data["category"], data["open_count"])
		}
		comp, _ := data["competition"].(map[string]any)
		if comp["views"] != float64(6) || comp["views_prev"] != float64(0) {
			t.Errorf("competition views = %v / %v, want 6 / 0", comp["views"], comp["views_prev"])
		}
		if want := now.AddDate(0, 0, -1).Format("2006-01-02"); comp["views_through"] != want {
			t.Errorf("views_through = %v, want %s", comp["views_through"], want)
		}
		if e, _ := comp["exposure_days"].(float64); e <= 0 {
			t.Errorf("exposure_days = %v, want > 0", comp["exposure_days"])
		}
		ttc, _ := data["time_to_close"].(map[string]any)
		if ttc["closed"] != float64(1) || ttc["median_days"] == nil || ttc["median_days_prev"] != nil {
			t.Errorf("time_to_close = %v, want closed 1, a median, no earlier median", ttc)
		}
		inflow, _ := data["inflow"].(map[string]any)
		if inflow["count"] != float64(4) {
			t.Errorf("inflow = %v, want count 4", inflow)
		}
		if _, ok := inflow["count_prev"]; ok {
			t.Errorf("inflow carries count_prev %v; the earlier window is deliberately not served", inflow)
		}
		if strings.Contains(raw, "SECRET_") || strings.Contains(raw, "secret-") {
			t.Errorf("record-level field leaked: %s", raw)
		}
	})

	t.Run("category without data answers zeros", func(t *testing.T) {
		out, raw, code := getInsights(t, ctx, app, "/api/v1/insights/role-pulse?category=design")
		if code != fiber.StatusOK {
			t.Fatalf("status %d, want 200: %s", code, raw)
		}
		data, _ := out["data"].(map[string]any)
		ttc, _ := data["time_to_close"].(map[string]any)
		comp, _ := data["competition"].(map[string]any)
		if data["open_count"] != float64(0) || ttc["closed"] != float64(0) || ttc["median_days"] != nil || comp["views_through"] != nil {
			t.Errorf("empty category = %s, want zeros and nulls", raw)
		}
	})

	for _, path := range []string{
		"/api/v1/insights/role-pulse",
		"/api/v1/insights/role-pulse?category=not-a-category",
	} {
		if _, raw, code := getInsights(t, ctx, app, path); code != fiber.StatusBadRequest {
			t.Errorf("%s: status %d, want 400: %s", path, code, raw)
		}
	}
}
