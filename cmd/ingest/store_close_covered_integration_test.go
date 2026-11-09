//go:build integration

// Integration test for CloseCovered: closing the stored aggregator copies of postings whose
// employer the coverage gate found already crawled first-hand.
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestCloseCovered_ClosesOpenRowsAndQueuesTheirRemoval(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy())
	for _, id := range []string{"covered:open", "covered:other"} {
		if err := store.Save(ctx, cheapPosting(id, "Backend Engineer")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	crawled := newCrawledSet()
	store.crawled = crawled

	slugs, err := store.CloseCovered(ctx, "lever", []string{"covered:open", "covered:never-stored"})
	if err != nil {
		t.Fatalf("CloseCovered: %v", err)
	}
	row := loadJob(t, pool, "covered:open")
	if len(slugs) != 1 || slugs[0] != row.CompanySlug {
		t.Errorf("closed company slugs = %v, want only %q", slugs, row.CompanySlug)
	}
	if !row.ClosedAt.Valid || row.ClosedReason != "ats_covered" {
		t.Errorf("row closed_at=%v reason=%q, want closed as ats_covered", row.ClosedAt, row.ClosedReason)
	}
	if loadJob(t, pool, "covered:other").ClosedAt.Valid {
		t.Error("a row the board did not list as covered was closed")
	}
	var queued int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM search_delete_outbox WHERE job_id = $1`, row.ID).Scan(&queued); err != nil {
		t.Fatalf("outbox: %v", err)
	}
	if queued != 1 {
		t.Errorf("search_delete_outbox rows = %d, want 1", queued)
	}
	if got := crawled.slugs("lever"); !slices.Equal(got, []string{row.CompanySlug}) {
		t.Errorf("crawled = %v, want [%s]", got, row.CompanySlug)
	}

	// Coverage lapsed: the aggregator posting is written again and the row reopens.
	if err := store.Save(ctx, cheapPosting("covered:open", "Backend Engineer")); err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	if loadJob(t, pool, "covered:open").ClosedAt.Valid {
		t.Error("re-saving the posting did not reopen it")
	}
}
