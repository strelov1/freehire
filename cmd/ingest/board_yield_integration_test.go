//go:build integration

// Integration tests for board_health.last_yield_at (migration 0158): the column that tells a
// board whose feed carries nothing apart from one whose crawls simply fail — a distinction
// last_success_at and consecutive_failures cannot make, because an empty-but-reachable feed
// refreshes both on every run.
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func lastYieldAt(t *testing.T, pool *pgxpool.Pool, provider, board string) (time.Time, bool) {
	t.Helper()
	var ts *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT last_yield_at FROM board_health WHERE provider = $1 AND board = $2 AND region = ''`,
		provider, board).Scan(&ts); err != nil {
		t.Fatalf("read last_yield_at for %s/%s: %v", provider, board, err)
	}
	if ts == nil {
		return time.Time{}, false
	}
	return *ts, true
}

// A crawl that reached a posting stamps the yield; one that reached nothing does not, even
// though both are recorded as SUCCESSES. This is the whole mechanism in one test.
func TestRecordSuccessStampsYieldOnlyWhenTheCrawlReachedAPosting(t *testing.T) {
	pool := startPostgres(t)
	h := newBoardHealth(pool)
	ctx := context.Background()

	if err := h.RecordSuccess(ctx, "whatjobs-hu", "empty-keyword", "", 0, false); err != nil {
		t.Fatalf("RecordSuccess (empty): %v", err)
	}
	if _, ok := lastYieldAt(t, pool, "whatjobs-hu", "empty-keyword"); ok {
		t.Error("last_yield_at was stamped for a crawl that reached nothing — an empty feed would " +
			"then be indistinguishable from a live one, which is the bug this column exists for")
	}

	if err := h.RecordSuccess(ctx, "whatjobs-at", "softwareentwickler", "", 0, true); err != nil {
		t.Fatalf("RecordSuccess (reached): %v", err)
	}
	// ingested = 0 deliberately: on an aggregator board whose employers we already crawl
	// directly, every posting can be reached and none of them written. Stamping on the ingested
	// count instead of on `reached` would call that board empty.
	if _, ok := lastYieldAt(t, pool, "whatjobs-at", "softwareentwickler"); !ok {
		t.Error("last_yield_at was not stamped for a crawl that reached postings but wrote none")
	}
}

// A later empty run must not CLEAR an earlier yield: the column is a high-water mark, and how
// long ago it was set is the entire measurement. Clearing it would restart every board's clock
// on its first quiet day.
func TestRecordSuccessKeepsAnEarlierYieldThroughALaterEmptyRun(t *testing.T) {
	pool := startPostgres(t)
	h := newBoardHealth(pool)
	ctx := context.Background()

	if err := h.RecordSuccess(ctx, "greenhouse", "acme-yield", "", 3, true); err != nil {
		t.Fatalf("RecordSuccess (reached): %v", err)
	}
	stamped, ok := lastYieldAt(t, pool, "greenhouse", "acme-yield")
	if !ok {
		t.Fatal("fixture: the first run should have stamped a yield")
	}

	if err := h.RecordSuccess(ctx, "greenhouse", "acme-yield", "", 0, false); err != nil {
		t.Fatalf("RecordSuccess (empty): %v", err)
	}
	after, ok := lastYieldAt(t, pool, "greenhouse", "acme-yield")
	if !ok {
		t.Fatal("a later empty run cleared last_yield_at; it must be left exactly as it was")
	}
	if !after.Equal(stamped) {
		t.Errorf("last_yield_at moved from %v to %v on an empty run; it must not change at all", stamped, after)
	}
}
