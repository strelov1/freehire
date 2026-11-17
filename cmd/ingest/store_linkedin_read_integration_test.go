//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestLinkedInReadCache_SeenWithinWindowOnly(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())

	for _, id := range []string{"111", "222", "333"} {
		if err := store.RecordLinkedInRead(ctx, id, "external"); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	// 222 was read 5 days ago (outside the 3-day default), 333 40 days ago (pruned).
	if _, err := pool.Exec(ctx, `UPDATE linkedin_postings_read SET read_at = now() - CASE posting_id
		WHEN '222' THEN interval '5 days' WHEN '333' THEN interval '40 days' ELSE interval '0' END`); err != nil {
		t.Fatal(err)
	}
	set, err := store.ExistingExternalIDs(ctx, "linkedin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["111"]; !ok {
		t.Error("a posting read just now is not seen")
	}
	if _, ok := set["222"]; ok {
		t.Error("a posting read 5 days ago is seen under the 3-day default window")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM linkedin_postings_read WHERE posting_id = '333'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("stale row not pruned: n=%d err=%v", n, err)
	}

	// With BODY_REFRESH_DAYS set the cache follows the store's own cutoff.
	br, err := sweeppolicy.BodyRefreshFor("7", "1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	policy := defaultSeenPolicy()
	policy.bodies = br
	set, err = newDBStore(pool, 1, newCrawledSet(), nil, policy).ExistingExternalIDs(ctx, "linkedin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["222"]; !ok {
		t.Error("a 5-day-old read is not seen under BODY_REFRESH_DAYS=7")
	}

	// Other providers never see the cache.
	other, err := store.ExistingExternalIDs(ctx, "justjoin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := other["111"]; ok {
		t.Error("the linkedin read cache leaked into another provider's seen-set")
	}
}

// A cached posting is emitted as a SeenRefresh, but has no row: touching it must be a quiet no-op.
func TestTouch_LinkedInPostingWithoutRowIsANoOp(t *testing.T) {
	pool := testdb.Pool(t)
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())
	slug, err := store.Touch(context.Background(), "linkedin", "999", nil)
	if err != nil || slug != "" {
		t.Fatalf("Touch of an absent linkedin row = %q, %v; want a quiet no-op", slug, err)
	}
}
