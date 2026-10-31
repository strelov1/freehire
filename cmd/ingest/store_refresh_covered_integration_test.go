//go:build integration

// Integration test for RefreshCovered: the liveness refresh for the postings an aggregator board
// listed but the coverage gate kept out of the write path.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestRefreshCovered_RefreshesOpenStoredRowsOnly(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	// No crawled set while seeding, so the one checked below holds only what the refresh records.
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())

	// Seeded as lever rows (cheapPosting's source) and refreshed as such: the query is keyed on
	// (source, external_id) alone, so which provider stands in for the aggregator is immaterial.
	for _, id := range []string{"covered:open", "covered:closed"} {
		if err := store.Save(ctx, cheapPosting(id, "Backend Engineer")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
		backdateStamps(t, pool, id)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now() - interval '1 day', closed_reason = 'unseen'
		 WHERE source = 'lever' AND external_id = 'covered:closed'`); err != nil {
		t.Fatalf("close: %v", err)
	}
	crawled := newCrawledSet()
	store.crawled = crawled
	before := loadJob(t, pool, "covered:open")

	n, err := store.RefreshCovered(ctx, "lever", []string{"covered:open", "covered:closed", "covered:never-stored"})
	if err != nil {
		t.Fatalf("RefreshCovered: %v", err)
	}
	if n != 1 {
		t.Errorf("refreshed %d rows, want 1 — the open stored one", n)
	}

	open := loadJob(t, pool, "covered:open")
	if time.Since(open.LastSeenAt.Time) > time.Minute {
		t.Errorf("open row last_seen_at = %v, want refreshed to now", open.LastSeenAt.Time)
	}
	if !open.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Errorf("updated_at moved %v -> %v; the refresh writes liveness and nothing else",
			before.UpdatedAt.Time, open.UpdatedAt.Time)
	}
	closed := loadJob(t, pool, "covered:closed")
	if !closed.ClosedAt.Valid {
		t.Error("a closed row was reopened; the gate declined to write it, so nothing may reopen it")
	}
	if time.Since(closed.LastSeenAt.Time) < 24*time.Hour {
		t.Errorf("closed row last_seen_at = %v, want untouched", closed.LastSeenAt.Time)
	}
	// The refreshed row's company joins the sweep's scope, so its siblings the aggregator no
	// longer lists are closed by the ordinary company-scoped sweep.
	if got := crawled.slugs("lever"); !slices.Equal(got, []string{open.CompanySlug}) {
		t.Errorf("crawled = %v, want [%s]", got, open.CompanySlug)
	}
}
