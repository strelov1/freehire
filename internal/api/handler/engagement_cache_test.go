package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A cached row is served without touching the database: queries is nil here, so a
// read that fell through to it would panic instead of answering.
func TestEngagementStatsServedFromCache(t *testing.T) {
	ctx := context.Background()
	mem := cache.NewMemory()
	row := db.GetEngagementStatsRow{
		Saved:       7,
		Viewed:      42,
		ViewedSince: pgtype.Date{Time: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	if err := cache.SetJSON(ctx, mem, engagementCacheKey, row, time.Minute); err != nil {
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
		t.Fatalf("got %+v, want the cached row", env.Data)
	}
}
