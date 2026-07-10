package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// fixedNow is a mid-day UTC instant; the parser must truncate `to` to the calendar
// date, so every default-`to` assertion below expects 2026-07-10 00:00 UTC.
var fixedNow = time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestParseActivityQuery(t *testing.T) {
	t.Run("defaults to a 90-day daily window ending today", func(t *testing.T) {
		q, err := parseActivityQuery("", "", "", fixedNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.Granularity != "day" {
			t.Errorf("granularity = %q, want day", q.Granularity)
		}
		if want := date(2026, 7, 10); !q.To.Equal(want) {
			t.Errorf("to = %v, want %v (today, truncated)", q.To, want)
		}
		if want := date(2026, 4, 11); !q.From.Equal(want) {
			t.Errorf("from = %v, want %v (to - 90d)", q.From, want)
		}
	})

	t.Run("week granularity widens the default window to 52 weeks", func(t *testing.T) {
		q, err := parseActivityQuery("week", "", "", fixedNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.Granularity != "week" {
			t.Errorf("granularity = %q, want week", q.Granularity)
		}
		if want := date(2025, 7, 11); !q.From.Equal(want) {
			t.Errorf("from = %v, want %v (to - 52w)", q.From, want)
		}
	})

	t.Run("month granularity widens the default window to 24 months", func(t *testing.T) {
		q, err := parseActivityQuery("month", "", "", fixedNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := date(2024, 7, 10); !q.From.Equal(want) {
			t.Errorf("from = %v, want %v (to - 24mo)", q.From, want)
		}
	})

	t.Run("explicit from/to override the defaults", func(t *testing.T) {
		q, err := parseActivityQuery("day", "2026-01-01", "2026-02-01", fixedNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := date(2026, 1, 1); !q.From.Equal(want) {
			t.Errorf("from = %v, want %v", q.From, want)
		}
		if want := date(2026, 2, 1); !q.To.Equal(want) {
			t.Errorf("to = %v, want %v", q.To, want)
		}
	})

	t.Run("unknown granularity is an error", func(t *testing.T) {
		if _, err := parseActivityQuery("hour", "", "", fixedNow); err == nil {
			t.Fatal("expected error for granularity=hour, got nil")
		}
	})

	t.Run("malformed date is an error", func(t *testing.T) {
		if _, err := parseActivityQuery("day", "01-01-2026", "", fixedNow); err == nil {
			t.Fatal("expected error for malformed from date, got nil")
		}
	})

	t.Run("from after to is an error", func(t *testing.T) {
		if _, err := parseActivityQuery("day", "2026-02-01", "2026-01-01", fixedNow); err == nil {
			t.Fatal("expected error when from is after to, got nil")
		}
	})

	t.Run("an absurdly large range is rejected", func(t *testing.T) {
		if _, err := parseActivityQuery("day", "1900-01-01", "2026-01-01", fixedNow); err == nil {
			t.Fatal("expected error for an over-long range, got nil")
		}
	})

	t.Run("a multi-millennium range is rejected without duration overflow", func(t *testing.T) {
		// A span this wide overflows time.Duration; the cap must still reject it
		// rather than wrap negative and let a giant generate_series through.
		if _, err := parseActivityQuery("day", "0001-01-01", "9999-12-31", fixedNow); err == nil {
			t.Fatal("expected error for a multi-millennium range, got nil")
		}
	})
}

// TestJobsActivityValidation covers the DB-free path of the handler: the route is
// public (no auth middleware, so an unauthenticated request reaches the handler
// rather than being turned away with 401), and an invalid granularity is rejected
// with 400 before any query runs. The envelope shape and aggregation are covered
// by the integration test against a real Postgres.
func TestJobsActivityValidation(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h := &API{}
	app.Get("/api/v1/stats/jobs-activity", h.JobsActivity)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/stats/jobs-activity?granularity=hour", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unauthenticated request reaches the handler and invalid granularity is a 400)", resp.StatusCode)
	}
}
