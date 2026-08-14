package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func unhealthyRow(provider, board, region string, fails int32, cooledUntil time.Time) db.ListUnhealthyBoardsRow {
	r := db.ListUnhealthyBoardsRow{Provider: provider, Board: board, Region: region, ConsecutiveFailures: fails}
	if !cooledUntil.IsZero() {
		r.CooldownUntil = pgtype.Timestamptz{Time: cooledUntil, Valid: true}
	}
	return r
}

// The whole point of the cap: a fleet with thousands of unhealthy boards must still produce a
// bounded line, and it must say how many it left out so the number is never mistaken for the total.
func TestUnhealthyBoardsSummaryTruncates(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	rows := []db.ListUnhealthyBoardsRow{
		unhealthyRow("personio", "globus-ai", "", 38, now.Add(time.Hour)),
		unhealthyRow("deel", "deel", "", 37, time.Time{}),
	}
	got := unhealthyBoardsSummary(rows, 7397, now)
	for _, want := range []string{
		"7397 unhealthy board(s)",
		"worst 2",
		"personio/globus-ai(fails=38,cooled_until=2026-08-14T13:00:00Z)",
		"deel/deel(fails=37)",
		"7395 more",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
}

// Nothing was left out, so neither the "worst N" qualifier nor the remainder belongs in the line.
func TestUnhealthyBoardsSummaryFitsWhole(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	rows := []db.ListUnhealthyBoardsRow{unhealthyRow("ashby", "AtomicSemi", "", 3, time.Time{})}
	got := unhealthyBoardsSummary(rows, 1, now)
	if want := "1 unhealthy board(s): ashby/AtomicSemi(fails=3)"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// A region separates one board id repeated across regional slices (Adzuna), so it stays in the id.
func TestUnhealthyBoardsSummaryKeepsRegion(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	rows := []db.ListUnhealthyBoardsRow{unhealthyRow("adzuna", "it-jobs", "gb", 2, time.Time{})}
	if got := unhealthyBoardsSummary(rows, 1, now); !strings.Contains(got, "adzuna/it-jobs/gb(fails=2)") {
		t.Errorf("summary lost the region slice: %s", got)
	}
}

// A cooldown that has already lapsed says nothing about the board's current state — the failure
// count already carries that — so it is not printed.
func TestUnhealthyBoardsSummaryDropsLapsedCooldown(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	rows := []db.ListUnhealthyBoardsRow{unhealthyRow("lever", "acme", "", 1, now.Add(-time.Hour))}
	if got := unhealthyBoardsSummary(rows, 1, now); strings.Contains(got, "cooled_until") {
		t.Errorf("summary printed a lapsed cooldown: %s", got)
	}
}
