//go:build integration

// Integration test for the member-growth read endpoint. The cumulative series is
// pure SQL over users.created_at and the handler reads through a concrete
// *db.Queries, so it can only be exercised against a real Postgres. It asserts the
// empty-catalogue case, then seeds registrations on controlled UTC days and checks
// the dense, gap-filled, monotonically non-decreasing cumulative series.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

func TestUserGrowthEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/user-growth", h.UserGrowth)

	type point struct {
		Date  string `json:"date"`
		Total int    `json:"total"`
	}
	type envelope struct {
		Data []point `json:"data"`
	}
	get := func() envelope {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/stats/user-growth", nil))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200 (public, unauthenticated read)", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env
	}

	// --- Empty catalogue: 200 with an empty (non-null) series ------------------
	if empty := get(); len(empty.Data) != 0 {
		t.Fatalf("empty catalogue: got %d points, want 0", len(empty.Data))
	}

	// --- Seed registrations on controlled UTC days -----------------------------
	// mid-day, to prove UTC-date bucketing regardless of session timezone.
	seed := func(email string, y int, m time.Month, d int) {
		created := time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (email, created_at) VALUES ($1, $2)`, email, created); err != nil {
			t.Fatalf("seed user %q: %v", email, err)
		}
	}
	// 2026-01-05: 2 members; 2026-01-10: 3 members. Cumulative: 2 from 01-05,
	// flat through 01-09, 5 from 01-10 onward. Total = 5.
	seed("a@example.test", 2026, 1, 5)
	seed("b@example.test", 2026, 1, 5)
	seed("c@example.test", 2026, 1, 10)
	seed("d@example.test", 2026, 1, 10)
	seed("e@example.test", 2026, 1, 10)

	series := get().Data
	if len(series) == 0 {
		t.Fatal("seeded series is empty")
	}

	byDate := map[string]int{}
	prev := 0
	for i, p := range series {
		byDate[p.Date] = p.Total
		if p.Total < prev {
			t.Errorf("series not monotonic at %s: %d < previous %d", p.Date, p.Total, prev)
		}
		prev = p.Total
		if i == 0 && p.Date != "2026-01-05" {
			t.Errorf("series starts at %s, want 2026-01-05 (first registration day)", p.Date)
		}
	}

	if got := byDate["2026-01-05"]; got != 2 {
		t.Errorf("2026-01-05 total = %d, want 2", got)
	}
	if got := byDate["2026-01-07"]; got != 2 { // gap day repeats the running total
		t.Errorf("2026-01-07 total = %d, want 2 (flat gap day)", got)
	}
	if got := byDate["2026-01-10"]; got != 5 {
		t.Errorf("2026-01-10 total = %d, want 5", got)
	}
	if last := series[len(series)-1].Total; last != 5 {
		t.Errorf("final total = %d, want 5 (all seeded members)", last)
	}
}
