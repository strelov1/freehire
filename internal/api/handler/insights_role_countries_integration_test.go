//go:build integration

// Integration test for GET /api/v1/insights/role-countries, the role × country grid.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestInsightsRoleCountriesEndpoint(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Twelve countries, the n-th holding n backend postings, so the volume order is
	// c12, c11, … and the default ten columns drop c01 and c02. Codes are two letters
	// ("ca".."cl") because jobs.countries holds ISO alpha-2.
	codes := "abcdefghijkl"
	for i := 1; i <= len(codes); i++ {
		code := "c" + string(codes[i-1])
		for j := 0; j < i; j++ {
			seedInsightsHandlerJob(t, ctx, pool, q, fmt.Sprintf("%s-%d", code, j), "backend", "", []string{code}, []string{}, 0)
		}
	}
	// One design posting in the largest country, so a second row exists.
	seedInsightsHandlerJob(t, ctx, pool, q, "design-cl", "design", "", []string{"cl"}, []string{}, 0)

	if err := q.DeleteAllInsightsRoleCountryStats(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	prev := pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -30), Valid: true}
	if _, err := q.RebuildInsightsRoleCountryStatsGlobal(ctx, prev); err != nil {
		t.Fatalf("rebuild global: %v", err)
	}
	if _, err := q.RebuildInsightsRoleCountryStatsByCountry(ctx, prev); err != nil {
		t.Fatalf("rebuild by country: %v", err)
	}

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/role-countries", h.InsightsRoleCountries)

	t.Run("default grid", func(t *testing.T) {
		out, raw, code := getInsights(t, ctx, app, "/api/v1/insights/role-countries")
		if code != fiber.StatusOK {
			t.Fatalf("status %d, want 200: %s", code, raw)
		}
		meta, _ := out["meta"].(map[string]any)
		cols, _ := meta["countries"].([]any)
		wantCols := []any{"cl", "ck", "cj", "ci", "ch", "cg", "cf", "ce", "cd", "cc"}
		if fmt.Sprint(cols) != fmt.Sprint(wantCols) {
			t.Errorf("meta.countries = %v, want %v", cols, wantCols)
		}
		cells := cellIndex(t, out["data"])
		// all-countries: 1+2+…+12 = 78 backend postings, 1 design posting.
		if cells["backend/"] != 78 || cells["design/"] != 1 {
			t.Errorf("all-countries cells = backend %v, design %v; want 78, 1", cells["backend/"], cells["design/"])
		}
		if cells["backend/cl"] != 12 || cells["design/cl"] != 1 {
			t.Errorf("cl cells = backend %v, design %v; want 12, 1", cells["backend/cl"], cells["design/cl"])
		}
		for key := range cells {
			if strings.HasSuffix(key, "/ca") || strings.HasSuffix(key, "/cb") {
				t.Errorf("cell %s is outside the ten columns", key)
			}
		}
		// Aggregate-only: nothing record-level from the seeds reaches the body.
		for _, leak := range []string{"SECRET_TITLE_", "secret-ext-", "secret-slug-", "ex.test"} {
			if strings.Contains(raw, leak) {
				t.Errorf("body leaks %q: %s", leak, raw)
			}
		}
	})

	t.Run("fewer columns", func(t *testing.T) {
		out, raw, code := getInsights(t, ctx, app, "/api/v1/insights/role-countries?countries=3")
		if code != fiber.StatusOK {
			t.Fatalf("status %d, want 200: %s", code, raw)
		}
		meta, _ := out["meta"].(map[string]any)
		if got := fmt.Sprint(meta["countries"]); got != "[cl ck cj]" {
			t.Errorf("meta.countries = %s, want [cl ck cj]", got)
		}
		cells := cellIndex(t, out["data"])
		if cells["backend/cj"] != 10 {
			t.Errorf("backend/cj = %v, want 10 (the third column must carry its cells)", cells["backend/cj"])
		}
		for key := range cells {
			country := key[strings.Index(key, "/")+1:]
			if country != "" && country != "cl" && country != "ck" && country != "cj" {
				t.Errorf("cell %s is outside the three columns", key)
			}
		}
	})

	t.Run("empty rollup", func(t *testing.T) {
		if err := q.DeleteAllInsightsRoleCountryStats(ctx); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, raw, code := getInsights(t, ctx, app, "/api/v1/insights/role-countries")
		if code != fiber.StatusOK {
			t.Fatalf("status %d, want 200: %s", code, raw)
		}
		// Arrays, never null: the page decides to hide the grid from an empty data.
		if !strings.Contains(raw, `"data":[]`) || !strings.Contains(raw, `"countries":[]`) {
			t.Errorf("empty rollup body = %s, want empty arrays for data and meta.countries", raw)
		}
	})

	for _, bad := range []string{"0", "-1", "26", "ten"} {
		t.Run("rejects countries="+bad, func(t *testing.T) {
			if _, raw, code := getInsights(t, ctx, app, "/api/v1/insights/role-countries?countries="+bad); code != fiber.StatusBadRequest {
				t.Errorf("status %d, want 400: %s", code, raw)
			}
		})
	}
}

// cellIndex keys the response rows "category/country" → open_count, failing on a row
// that does not carry the four documented fields.
func cellIndex(t *testing.T, data any) map[string]float64 {
	t.Helper()
	rows, ok := data.([]any)
	if !ok {
		t.Fatalf("data is %T, want an array", data)
	}
	out := map[string]float64{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		cat, okCat := row["category"].(string)
		country, okCountry := row["country"].(string)
		open, okOpen := row["open_count"].(float64)
		_, okPrev := row["open_count_prev"].(float64)
		if !okCat || !okCountry || !okOpen || !okPrev || len(row) != 4 {
			t.Fatalf("row %v does not carry exactly category, country, open_count, open_count_prev", row)
		}
		out[cat+"/"+country] = open
	}
	return out
}
