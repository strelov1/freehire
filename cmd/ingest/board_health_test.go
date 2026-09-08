package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
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

func chronicRow(provider, board string, sinceEvidence time.Time, neverSucceeded bool) db.ListChronicBoardsRow {
	r := db.ListChronicBoardsRow{Provider: provider, Board: board, FirstSeenAt: pgtype.Timestamptz{Time: sinceEvidence, Valid: true}}
	if !neverSucceeded {
		r.LastSuccessAt = pgtype.Timestamptz{Time: sinceEvidence, Valid: true}
	}
	return r
}

// chronicBoardsSummary (openspec change close-chronically-unreachable-boards, issue #2017)
// must render as its own line, distinct from unhealthyBoardsSummary's — a curator scanning the
// run log needs to tell "still within an ordinary backoff" apart from "has not worked in over a
// month" (ingest-board-health spec, "The unhealthy-board summary distinguishes chronic boards").
func TestChronicBoardsSummaryIsDistinctFromUnhealthy(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rows := []db.ListChronicBoardsRow{chronicRow("paylocity", "3d3c12d8", now.Add(-41*24*time.Hour), false)}
	got := chronicBoardsSummary(rows, 1, now)
	if !strings.Contains(got, "chronic") {
		t.Errorf("summary does not identify itself as the chronic group: %s", got)
	}
	if strings.Contains(got, "unhealthy board(s)") {
		t.Errorf("chronic summary reused the unhealthy-board wording verbatim: %s", got)
	}
}

// The days-since-evidence figure is what tells a curator how bad a chronic board is at a
// glance, without computing it from a raw timestamp.
func TestChronicBoardsSummaryReportsDaysSinceEvidence(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rows := []db.ListChronicBoardsRow{chronicRow("paylocity", "3d3c12d8", now.Add(-41*24*time.Hour), false)}
	if got := chronicBoardsSummary(rows, 1, now); !strings.Contains(got, "paylocity/3d3c12d8(days=41)") {
		t.Errorf("summary missing days-since-evidence: %s", got)
	}
}

// A board that has never once succeeded has no last_success_at to report from — the summary
// must fall back to first_seen_at rather than mis-measuring it as "always chronic".
func TestChronicBoardsSummaryHandlesNeverSucceeded(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rows := []db.ListChronicBoardsRow{chronicRow("acme", "b1", now.Add(-35*24*time.Hour), true)}
	if got := chronicBoardsSummary(rows, 1, now); !strings.Contains(got, "acme/b1(days=35)") {
		t.Errorf("summary missing never-succeeded board's days-since-first-seen: %s", got)
	}
}

// A boardless provider's chronic record has no board id to append — the line must read
// "provider(days=N)", not "provider/(days=N)" with a dangling separator.
func TestChronicBoardsSummaryOmitsSlashForBoardlessProvider(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rows := []db.ListChronicBoardsRow{chronicRow("uber", "", now.Add(-61*24*time.Hour), false)}
	got := chronicBoardsSummary(rows, 1, now)
	if !strings.Contains(got, "uber(days=61)") {
		t.Errorf("summary missing boardless provider's entry: %s", got)
	}
	if strings.Contains(got, "uber/") {
		t.Errorf("summary should not carry a dangling slash for a boardless provider: %s", got)
	}
}

// Truncation follows the same cap/total convention as unhealthyBoardsSummary — a chronic
// backlog must still produce a bounded line.
func TestChronicBoardsSummaryTruncates(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rows := []db.ListChronicBoardsRow{chronicRow("paylocity", "3d3c12d8", now.Add(-41*24*time.Hour), false)}
	got := chronicBoardsSummary(rows, 5, now)
	for _, want := range []string{"5 chronic board(s)", "worst 1", "4 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
}
