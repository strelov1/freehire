//go:build integration

// Integration test for GET /api/v1/insights/salary/history.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestInsightsSalaryHistoryEndpoint(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE insights_salary_monthly"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// The rollup is tested in internal/platform/db; here the table is seeded directly so
	// the test pins the wire contract, not the percentile arithmetic again.
	if _, err := pool.Exec(ctx, `
		INSERT INTO insights_salary_monthly (category, currency, period, month, sample_size, p25, p50, p75) VALUES
			('backend', 'USD', 'year',  '2026-09-01', 40, 90000, 110000, 130000),
			('backend', 'USD', 'year',  '2026-08-01', 35, 88000, 108000, 128000),
			('backend', 'EUR', 'month', '2026-09-01', 31, 4000, 5000, 6000),
			('design',  'USD', 'year',  '2026-09-01', 50, 70000, 80000, 90000)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/salary/history", h.InsightsSalaryHistory)

	out, raw, code := getInsights(t, ctx, app, "/api/v1/insights/salary/history?category=backend")
	if code != fiber.StatusOK {
		t.Fatalf("status %d, want 200: %s", code, raw)
	}
	rows, _ := out["data"].([]any)
	var got []string
	for _, r := range rows {
		row, _ := r.(map[string]any)
		got = append(got, row["currency"].(string)+"/"+row["period"].(string)+"/"+row["month"].(string))
		for _, k := range []string{"sample_size", "p25", "p50", "p75"} {
			if _, ok := row[k].(float64); !ok {
				t.Errorf("row %v lacks numeric %s", row, k)
			}
		}
	}
	// backend only, ordered by currency, period, month.
	want := "EUR/month/2026-09 USD/year/2026-08 USD/year/2026-09"
	if strings.Join(got, " ") != want {
		t.Errorf("rows = %v, want %s", got, want)
	}

	for _, bad := range []string{"", "?category=", "?category=not-a-category"} {
		if _, raw, code := getInsights(t, ctx, app, "/api/v1/insights/salary/history"+bad); code != fiber.StatusBadRequest {
			t.Errorf("%q: status %d, want 400: %s", bad, code, raw)
		}
	}
}
