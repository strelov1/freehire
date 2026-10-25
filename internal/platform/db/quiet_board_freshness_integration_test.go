//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// A board in its quiet pause is not crawled for up to 72h by design, so its last_success_at
// ages past the 48h freshness the public /status page and the silence alerts read. Both
// rollups must count a quiet board as fresh, or every provider whose boards are all quiet would
// read "down" for part of each cycle with no incident behind it.
func TestQuietBoardsCountAsFresh(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO boards (provider, board, company, status) VALUES ('stable', 'calm', 'Calm', 'active')`); err != nil {
		t.Fatalf("seed boards: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO board_health (provider, board, consecutive_failures, last_success_at, last_run_at, quiet_streak, quiet_until)
		 VALUES ('stable', 'calm', 0, now() - interval '60 hours', now() - interval '60 hours', 5, now() + interval '4 hours')`); err != nil {
		t.Fatalf("seed board_health: %v", err)
	}

	rollup, err := q.ProviderHealthRollup(ctx)
	if err != nil || len(rollup) != 1 {
		t.Fatalf("ProviderHealthRollup = (%v, %v), want one row", rollup, err)
	}
	if age := time.Since(rollup[0].LastSuccessAt.Time); age > time.Minute {
		t.Errorf("/status rollup: quiet board's freshness is %v old, want ~now", age)
	}

	health, err := q.ProviderIngestHealth(ctx)
	if err != nil || len(health) != 1 {
		t.Fatalf("ProviderIngestHealth = (%v, %v), want one row", health, err)
	}
	if age := time.Since(health[0].LastSuccessAt.Time); age > time.Minute {
		t.Errorf("metrics: quiet board's freshness is %v old, want ~now", age)
	}

	// Once the pause has ended, the board is due again and its real age shows.
	if _, err := pool.Exec(ctx, `UPDATE board_health SET quiet_until = now() - interval '1 minute'`); err != nil {
		t.Fatalf("end pause: %v", err)
	}
	rollup, err = q.ProviderHealthRollup(ctx)
	if err != nil || len(rollup) != 1 {
		t.Fatalf("ProviderHealthRollup = (%v, %v), want one row", rollup, err)
	}
	if age := time.Since(rollup[0].LastSuccessAt.Time); age < 59*time.Hour {
		t.Errorf("after the pause ended the freshness is %v old, want the real ~60h", age)
	}
}
