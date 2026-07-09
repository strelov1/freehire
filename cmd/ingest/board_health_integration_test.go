//go:build integration

// Integration test for the board-health adapter: the failure→cooldown→self-heal cycle
// against a real Postgres (the backoff policy applied to the DB-returned failure count).
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (%d)", err, len(scripts))
	}
	sort.Strings(scripts)
	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"), postgres.WithUsername("hire"), postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...), postgres.BasicWaitStrategies())
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
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
