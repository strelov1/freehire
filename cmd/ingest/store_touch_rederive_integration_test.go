//go:build integration

package main

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

// A touch writes no content, so a posting a re-listing reopens must be re-derived, or it comes
// back with whatever the dictionary read when it closed. An open row's touch stays a pure
// liveness write: re-deriving there would be backfill-derive's whole table on every crawl.
func TestTouch_ReopenReDerivesTheRow(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())

	for _, id := range []string{"acme:touch-rederive-closed", "acme:touch-rederive-open"} {
		if err := store.Save(ctx, cheapPosting(id, "Contract Backend Engineer (Outside IR35)")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
		if got := loadJob(t, pool, id).Engagement; !slices.Equal(got, []string{"ir35_outside"}) {
			t.Fatalf("%s engagement after Save = %v, want [ir35_outside] — the fixture no longer derives it", id, got)
		}
	}
	// Both rows as an older dictionary left them; only the first is closed.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET engagement = '{}',
		        closed_at = CASE WHEN external_id = 'acme:touch-rederive-closed' THEN now() END,
		        closed_reason = CASE WHEN external_id = 'acme:touch-rederive-closed' THEN 'unseen' ELSE '' END
		 WHERE source = 'lever' AND external_id IN ('acme:touch-rederive-closed', 'acme:touch-rederive-open')`,
	); err != nil {
		t.Fatalf("stale setup: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id IN (SELECT id FROM jobs
		 WHERE source = 'lever' AND external_id IN ('acme:touch-rederive-closed', 'acme:touch-rederive-open'))`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}

	for _, id := range []string{"acme:touch-rederive-closed", "acme:touch-rederive-open"} {
		if _, err := store.Touch(ctx, "lever", id, nil); err != nil {
			t.Fatalf("Touch %s: %v", id, err)
		}
	}

	reopened := loadJob(t, pool, "acme:touch-rederive-closed")
	if reopened.ClosedAt.Valid {
		t.Fatal("closed row still closed after the touch")
	}
	if !slices.Equal(reopened.Engagement, []string{"ir35_outside"}) {
		t.Errorf("reopened engagement = %v, want [ir35_outside] re-derived from the stored text", reopened.Engagement)
	}
	if n := searchOutboxCount(t, pool, reopened.ID); n != 1 {
		t.Errorf("reopened row has %d search_outbox entries, want 1", n)
	}
	if got := loadJob(t, pool, "acme:touch-rederive-open").Engagement; len(got) != 0 {
		t.Errorf("open row engagement = %v, want left alone by a liveness touch", got)
	}
}
