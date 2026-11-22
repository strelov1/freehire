package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/application/viewlog"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Cached counters and published view totals are served without touching the database:
// queries is nil here, so a read that fell through to it would panic instead of answering.
// The view totals in particular must never be summed on a request path while a snapshot
// exists — that scan timed the endpoint out dozens of times a day (2026-11-21).
func TestEngagementStatsServedFromCacheAndViewSnapshot(t *testing.T) {
	ctx := context.Background()
	mem := cache.NewMemory()
	if err := cache.SetJSON(ctx, mem, engagementCacheKey, db.GetEngagementStatsRow{Saved: 7}, time.Minute); err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	if err := viewlog.StoreTotals(ctx, mem, viewlog.Totals{Viewed: 42, ViewedSince: &since, ComputedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	app := statsTestApp(&statsHandlers{cache: mem})

	resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/stats/engagement", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		Data struct {
			Saved       int    `json:"saved"`
			Viewed      int    `json:"viewed"`
			ViewedSince string `json:"viewed_since"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Saved != 7 || env.Data.Viewed != 42 || env.Data.ViewedSince != "2026-09-04" {
		t.Fatalf("got %+v, want the cached counters and the published view totals", env.Data)
	}
}
