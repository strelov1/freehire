//go:build integration

// Integration test for the board-health adapter: the failure→cooldown→self-heal cycle
// against a real Postgres (the backoff policy applied to the DB-returned failure count).
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
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
	if _, cooled, err := h.Cooldown(ctx, "greenhouse", "acme"); err != nil || cooled {
		t.Fatalf("fresh board Cooldown = (_, %v, %v), want (_, false, nil)", cooled, err)
	}

	// The first two failures stay below the threshold — no cooldown, cron retries.
	for i := 0; i < 2; i++ {
		if err := h.RecordFailure(ctx, "greenhouse", "acme", "boom"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	if _, cooled, _ := h.Cooldown(ctx, "greenhouse", "acme"); cooled {
		t.Error("board should not be cooled below the threshold (2 failures)")
	}

	// The third consecutive failure crosses the threshold → cooldown is set.
	if err := h.RecordFailure(ctx, "greenhouse", "acme", "boom again"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	until, cooled, err := h.Cooldown(ctx, "greenhouse", "acme")
	if err != nil || !cooled {
		t.Fatalf("after 3 failures Cooldown = (%v, %v, %v), want a future cooldown", until, cooled, err)
	}
	if !until.After(time.Now()) {
		t.Errorf("cooldown_until %v is not in the future", until)
	}

	// A success self-heals: failure state cleared, cooldown gone.
	if err := h.RecordSuccess(ctx, "greenhouse", "acme", 7); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if _, cooled, _ := h.Cooldown(ctx, "greenhouse", "acme"); cooled {
		t.Error("a success must clear the cooldown (self-heal)")
	}

	// And it appears unhealthy no more.
	rows, err := db.New(pool).ListUnhealthyBoards(ctx)
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
			if err := h.RecordFailure(ctx, provider, board, "boom"); err != nil {
				t.Fatalf("RecordFailure %s/%s: %v", provider, board, err)
			}
		}
	}
	// breezy: b1, b2, b3 cooled in that order; b-ok is fresh (never failed).
	cool("breezy", "b1")
	cool("breezy", "b2")
	cool("breezy", "b3")
	if err := h.RecordSuccess(ctx, "breezy", "b-ok", 1); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	// join: an unrelated provider's cooled board, to prove clearing is provider-scoped.
	cool("join", "j1")

	// The limit caps the sample, soonest-to-expire first — b1, b2 cooled before b3.
	if got, err := h.CooledBoards(ctx, "breezy", 2); err != nil {
		t.Fatalf("CooledBoards: %v", err)
	} else if len(got) != 2 || got[0] != "b1" || got[1] != "b2" {
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
	if _, cooled, err := h.Cooldown(ctx, "join", "j1"); err != nil || !cooled {
		t.Errorf("join/j1 Cooldown = (_, %v, %v), want still cooled after clearing breezy", cooled, err)
	}
}
