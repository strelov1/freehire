//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestTouch_UnchangedListingDeadlineLeavesTheJobsTupleAlone(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := newDBStore(pool, 1, newCrawledSet(), newWriteTally(), defaultSeenPolicy())
	const externalID = "act:touch-same-deadline"
	seedEnriched(t, pool, store, externalID)
	deadline := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 3)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET application_deadline=$1::date
        WHERE source='himalayas' AND external_id=$2`, deadline, externalID); err != nil {
		t.Fatal(err)
	}
	before := jobsTuple(t, pool, externalID)

	if _, err := store.Touch(ctx, "himalayas", externalID, &deadline); err != nil {
		t.Fatal(err)
	}

	if after := jobsTuple(t, pool, externalID); after != before {
		t.Errorf("unchanged listing deadline moved jobs: before %+v, after %+v", before, after)
	}
	if age := activitySeenAge(t, pool, externalID); age > time.Minute {
		t.Errorf("last_seen_at is %s old after a touch", age)
	}
}
