//go:build integration

// Integration test for the catalogue-size history: the two writers' precedence rules (a
// measurement replaces a reconstruction, a reconstruction never replaces anything) and the
// public read endpoint over them.
// Run with: go test -tags=integration ./internal/api/handler/
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

func TestCatalogHistoryEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)

	date := func(s string) pgtype.Date {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return pgtype.Date{Time: d, Valid: true}
	}
	measure := func(day string, n int64) {
		t.Helper()
		if err := q.RecordCatalogSize(ctx, db.RecordCatalogSizeParams{
			Day: date(day), OpenJobs: n, ComputedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}

	measure("2026-07-02", 300)
	if err := q.RecordReconstructedCatalogSize(ctx, db.RecordReconstructedCatalogSizeParams{
		Days:     []pgtype.Date{date("2026-07-01"), date("2026-07-02"), date("2026-07-03")},
		OpenJobs: []int64{100, 999, 777}, // 07-03 follows a measured day, so it is not written
	}); err != nil {
		t.Fatal(err)
	}
	measure("2026-07-01", 200) // a measurement replaces the reconstruction
	measure("2026-07-02", 350) // a later run the same day replaces the earlier one

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/catalog-history", statsContract(h))
	resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/stats/catalog-history", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var env struct {
		Data []struct {
			Date          string `json:"date"`
			OpenJobs      int64  `json:"open_jobs"`
			Reconstructed bool   `json:"reconstructed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data) != 2 ||
		env.Data[0].Date != "2026-07-01" || env.Data[0].OpenJobs != 200 || env.Data[0].Reconstructed ||
		env.Data[1].Date != "2026-07-02" || env.Data[1].OpenJobs != 350 || env.Data[1].Reconstructed {
		t.Fatalf("series = %+v", env.Data)
	}
}
