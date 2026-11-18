//go:build integration

// Integration test for INGEST_REFETCH_ALL: the repair switch empties the seen-set so every listed
// posting is re-fetched and re-written.
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestExistingExternalIDs_RefetchAllEmptiesTheSeenSet(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if err := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy()).Save(ctx, cheapPosting("acme:1", "Backend Engineer")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	seen, err := newDBStore(pool, 1, nil, newWriteTally(), defaultSeenPolicy()).ExistingExternalIDs(ctx, "lever", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := seen["acme:1"]; !ok {
		t.Fatalf("fixture: default policy seen-set = %v, want it to hold acme:1", seen)
	}

	repair := defaultSeenPolicy()
	repair.refetchAll = true
	seen, err = newDBStore(pool, 1, nil, newWriteTally(), repair).ExistingExternalIDs(ctx, "lever", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Errorf("refetchAll seen-set = %v, want empty", seen)
	}
}
