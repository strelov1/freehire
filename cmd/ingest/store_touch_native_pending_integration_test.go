//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

// A row closed as native_source_pending is withheld from publication because its application
// method is external or unknown. A touch only proves the posting was re-listed, which proves
// nothing about its application method, so it must leave such a row closed, with its reason,
// for every caller and for the reopen retry alike. Only a fresh detail read through the
// ordinary upsert may publish it.
func TestTouch_NeverReopensNativeSourcePending(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())

	const pending, unseen = "acme:touch-native-pending", "acme:touch-native-unseen"
	for _, id := range []string{pending, unseen} {
		if err := store.Save(ctx, cheapPosting(id, "Backend Engineer")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now(),
		        closed_reason = CASE WHEN external_id = $1 THEN 'native_source_pending' ELSE 'unseen' END
		 WHERE source = 'lever' AND external_id IN ($1, $2)`, pending, unseen,
	); err != nil {
		t.Fatalf("close setup: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id IN (SELECT id FROM jobs
		 WHERE source = 'lever' AND external_id IN ($1, $2))`, pending, unseen); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}

	future := time.Now().UTC().AddDate(0, 1, 0)
	for _, deadline := range []*time.Time{nil, &future} {
		if _, err := store.Touch(ctx, "lever", pending, deadline); err != nil {
			t.Fatalf("Touch pending (deadline %v): %v", deadline, err)
		}
		got := loadJob(t, pool, pending)
		if !got.ClosedAt.Valid || got.ClosedReason != "native_source_pending" {
			t.Fatalf("touch reopened a native_source_pending row: closed_at=%v reason=%q", got.ClosedAt, got.ClosedReason)
		}
		if n := searchOutboxCount(t, pool, got.ID); n != 0 {
			t.Fatalf("withheld row queued %d search_outbox entries, want 0", n)
		}
	}

	// The guard is scoped to the publication withhold: any other closure still reopens.
	if _, err := store.Touch(ctx, "lever", unseen, nil); err != nil {
		t.Fatalf("Touch unseen: %v", err)
	}
	if loadJob(t, pool, unseen).ClosedAt.Valid {
		t.Fatal("an ordinarily closed row no longer reopens on a touch")
	}
}
