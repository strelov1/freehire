//go:build integration

// Integration test for the site-status daily sampler: the background ticker
// cmd/server starts, which periodically records the site's current status
// into site_status_daily. A real Postgres is required because the sampler
// calls the real pool.Ping and the real RecordSiteStatusSample query.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestStartSiteStatusSampler(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	ctx, cancel := context.WithCancel(context.Background())

	const interval = 50 * time.Millisecond
	StartSiteStatusSampler(ctx, pool, queries, interval)

	readUpdatedAt := func(t *testing.T) time.Time {
		t.Helper()
		var updatedAt time.Time
		if err := pool.QueryRow(context.Background(),
			`SELECT updated_at FROM site_status_daily WHERE day = CURRENT_DATE`,
		).Scan(&updatedAt); err != nil {
			t.Fatalf("read site_status_daily: %v", err)
		}
		return updatedAt
	}

	// The first sample runs immediately, not after waiting out the first
	// interval — same shape as cmd/server's existing startSuggestRefresh.
	deadline := time.After(2 * time.Second)
	for {
		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM site_status_daily WHERE day = CURRENT_DATE`,
		).Scan(&count); err != nil {
			t.Fatalf("count site_status_daily: %v", err)
		}
		if count == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no row recorded within 2s of starting the sampler")
		case <-time.After(10 * time.Millisecond):
		}
	}
	firstUpdatedAt := readUpdatedAt(t)

	// A later tick re-samples and bumps updated_at, proving the sampler
	// actually repeats rather than firing once and stopping.
	time.Sleep(5 * interval)
	laterUpdatedAt := readUpdatedAt(t)
	if !laterUpdatedAt.After(firstUpdatedAt) {
		t.Errorf("updated_at did not advance after waiting %v: first=%v later=%v", 5*interval, firstUpdatedAt, laterUpdatedAt)
	}

	// Cancelling ctx stops the ticker: updated_at must not keep advancing.
	cancel()
	time.Sleep(2 * interval) // let any in-flight tick settle
	stoppedAt := readUpdatedAt(t)
	time.Sleep(5 * interval)
	afterStopAt := readUpdatedAt(t)
	if afterStopAt.After(stoppedAt) {
		t.Errorf("updated_at kept advancing after ctx was cancelled: stopped_at=%v after_stop=%v", stoppedAt, afterStopAt)
	}
}
