//go:build integration

// Integration test for the board-health adapter: the failure→cooldown→self-heal cycle
// against a real Postgres (the backoff policy applied to the DB-returned failure count).
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

func TestBoardHealth_FailureCooldownSelfHeal(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	// Never seen → eligible.
	if _, cooled, err := h.Cooldown(ctx, "greenhouse", "acme", ""); err != nil || cooled {
		t.Fatalf("fresh board Cooldown = (_, %v, %v), want (_, false, nil)", cooled, err)
	}

	// The first two failures stay below the threshold — no cooldown, cron retries.
	for i := 0; i < 2; i++ {
		if err := h.RecordFailure(ctx, "greenhouse", "acme", "", "boom"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if _, cooled, _ := h.Cooldown(ctx, "greenhouse", "acme", ""); cooled {
		t.Error("board should not be cooled below the threshold (2 failures)")
	}

	// The third consecutive failure crosses the threshold → cooldown is set.
	if err := h.RecordFailure(ctx, "greenhouse", "acme", "", "boom again"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	until, cooled, err := h.Cooldown(ctx, "greenhouse", "acme", "")
	if err != nil || !cooled {
		t.Fatalf("after 3 failures Cooldown = (%v, %v, %v), want a future cooldown", until, cooled, err)
	}
	if !until.After(time.Now()) {
		t.Errorf("cooldown_until %v is not in the future", until)
	}

	// A success self-heals: failure state cleared, cooldown gone.
	if err := h.RecordSuccess(ctx, "greenhouse", "acme", "", 7); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if _, cooled, _ := h.Cooldown(ctx, "greenhouse", "acme", ""); cooled {
		t.Error("a success must clear the cooldown (self-heal)")
	}

	// And it appears unhealthy no more.
	rows, err := db.New(pool).ListUnhealthyBoards(ctx, unhealthyBoardsCap)
	if err != nil {
		t.Fatalf("ListUnhealthyBoards: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("after self-heal, unhealthy boards = %d, want 0", len(rows))
	}
}

// The recovery-probe queries: CooledBoards lists only a provider's currently-cooled boards
// (soonest-to-expire first, honoring the limit), and ClearCooldowns clears exactly that
// provider's cooled boards — a fresh board and another provider's cooled board are both
// untouched.
func TestBoardHealth_CooledBoardsAndClear(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	cool := func(provider, board string) {
		t.Helper()
		for i := 0; i < 3; i++ { // crosses the cooldown threshold
			if err := h.RecordFailure(ctx, provider, board, "", "boom"); err != nil {
				t.Fatalf("RecordFailure %s/%s: %v", provider, board, err)
			}
		}
	}
	// breezy: b1, b2, b3 cooled in that order; b-ok is fresh (never failed).
	cool("breezy", "b1")
	cool("breezy", "b2")
	cool("breezy", "b3")
	if err := h.RecordSuccess(ctx, "breezy", "b-ok", "", 1); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	// join: an unrelated provider's cooled board, to prove clearing is provider-scoped.
	cool("join", "j1")

	// The limit caps the sample, soonest-to-expire first — b1, b2 cooled before b3.
	if got, err := h.CooledBoards(ctx, "breezy", 2); err != nil {
		t.Fatalf("CooledBoards: %v", err)
	} else if len(got) != 2 || got[0].Board != "b1" || got[1].Board != "b2" {
		t.Errorf("CooledBoards(breezy, 2) = %v, want [b1 b2]", got)
	}
	// Above the count: exactly the three cooled boards, never the fresh one.
	if got, err := h.CooledBoards(ctx, "breezy", 10); err != nil {
		t.Fatalf("CooledBoards: %v", err)
	} else if len(got) != 3 {
		t.Errorf("CooledBoards(breezy, 10) = %v, want the 3 cooled boards (not b-ok)", got)
	}

	// Clear breezy's cooldowns: exactly 3 rows, and none of its boards read cooled after.
	if n, err := h.ClearCooldowns(ctx, "breezy"); err != nil {
		t.Fatalf("ClearCooldowns: %v", err)
	} else if n != 3 {
		t.Errorf("ClearCooldowns(breezy) = %d, want 3", n)
	}
	if got, err := h.CooledBoards(ctx, "breezy", 10); err != nil {
		t.Fatalf("CooledBoards after clear: %v", err)
	} else if len(got) != 0 {
		t.Errorf("CooledBoards(breezy) after clear = %v, want none", got)
	}

	// join's cooled board is untouched — clearing is scoped to the one provider.
	if _, cooled, err := h.Cooldown(ctx, "join", "j1", ""); err != nil || !cooled {
		t.Errorf("join/j1 Cooldown = (_, %v, %v), want still cooled after clearing breezy", cooled, err)
	}
}

// A board id that repeats across independent regional slices (Adzuna's "it-jobs" once per
// country) must not share health state across regions: cooling one region's board leaves a
// same-named board in another region eligible, and CooledBoards/ClearCooldowns distinguish them.
func TestBoardHealth_RegionDisambiguates(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	for i := 0; i < 3; i++ { // crosses the cooldown threshold
		if err := h.RecordFailure(ctx, "adzuna", "it-jobs", "gb", "boom"); err != nil {
			t.Fatalf("RecordFailure gb: %v", err)
		}
	}
	if err := h.RecordSuccess(ctx, "adzuna", "it-jobs", "us", 42); err != nil {
		t.Fatalf("RecordSuccess us: %v", err)
	}

	if _, cooled, err := h.Cooldown(ctx, "adzuna", "it-jobs", "gb"); err != nil || !cooled {
		t.Fatalf("gb Cooldown = (_, %v, %v), want cooled", cooled, err)
	}
	if _, cooled, err := h.Cooldown(ctx, "adzuna", "it-jobs", "us"); err != nil || cooled {
		t.Fatalf("us Cooldown = (_, %v, %v), want eligible — gb's failures must not bleed into us", cooled, err)
	}

	got, err := h.CooledBoards(ctx, "adzuna", 10)
	if err != nil {
		t.Fatalf("CooledBoards: %v", err)
	}
	if len(got) != 1 || got[0].Board != "it-jobs" || got[0].Region != "gb" {
		t.Fatalf("CooledBoards(adzuna) = %v, want exactly [{it-jobs gb}]", got)
	}

	if n, err := h.ClearCooldowns(ctx, "adzuna"); err != nil || n != 1 {
		t.Fatalf("ClearCooldowns(adzuna) = (%d, %v), want (1, nil)", n, err)
	}
	if _, cooled, err := h.Cooldown(ctx, "adzuna", "it-jobs", "us"); err != nil || cooled {
		t.Fatalf("us Cooldown after clearing gb = (_, %v, %v), want still eligible", cooled, err)
	}
}

// The summary log's query is capped, but the count it reports must not be: count(*) OVER ()
// is evaluated over the whole filtered set, before the LIMIT. Getting that backwards would
// make every capped run report "N unhealthy boards" where N is just the cap — the exact
// misreading the cap was introduced to avoid.
func TestBoardHealth_ListUnhealthyBoardsCapsRowsNotCount(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	// Five unhealthy boards, descending failure counts so the ordering is unambiguous.
	for i, board := range []string{"b1", "b2", "b3", "b4", "b5"} {
		for j := 0; j <= 5-i; j++ {
			if err := h.RecordFailure(ctx, "greenhouse", board, "", "boom"); err != nil {
				t.Fatalf("RecordFailure %s: %v", board, err)
			}
		}
	}

	rows, err := db.New(pool).ListUnhealthyBoards(ctx, 2)
	if err != nil {
		t.Fatalf("ListUnhealthyBoards: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the 2 the limit allows", len(rows))
	}
	if rows[0].Total != 5 || rows[1].Total != 5 {
		t.Errorf("Total = (%d, %d), want 5 on every row — the count is pre-LIMIT",
			rows[0].Total, rows[1].Total)
	}
	// Worst first: b1 failed most.
	if rows[0].Board != "b1" || rows[1].Board != "b2" {
		t.Errorf("rows = [%s %s], want the two worst [b1 b2]", rows[0].Board, rows[1].Board)
	}

	// And the summary line built from them names two boards while reporting all five.
	got := unhealthyBoardsSummary(rows, rows[0].Total, time.Now())
	if !strings.Contains(got, "5 unhealthy board(s), worst 2") || !strings.Contains(got, "3 more") {
		t.Errorf("summary = %q, want it to report 5 total, name 2, and admit 3 more", got)
	}
}
