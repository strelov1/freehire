//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// With BODY_REFRESH_DAYS unset, LinkedIn still re-reads a stored posting once its body is
// older than the built-in window, so a posting closed on LinkedIn but still listed is noticed.
func TestExistingExternalIDs_LinkedInDefaultWindowRereadsStaleRows(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	// The seen-set query is source-agnostic; a lever fixture stands in for a stored row.
	store := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy())
	for _, id := range []string{"7001", "7002"} {
		if err := store.Save(ctx, cheapPosting(id, "Go Engineer")); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET hydrated_at = now() - interval '5 days'
		WHERE job_id = (SELECT id FROM jobs WHERE source='lever' AND external_id='7001')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET hydrated_at = now()
		WHERE job_id = (SELECT id FROM jobs WHERE source='lever' AND external_id='7002')`); err != nil {
		t.Fatal(err)
	}

	policy := defaultSeenPolicy()
	unset, err := sweeppolicy.BodyRefreshFor("", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	policy.bodies = unset.WithProviderDefault("linkedin", time.Now())
	set, err := newDBStore(pool, 1, newCrawledSet(), nil, policy).ExistingExternalIDs(ctx, "lever", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["7001"]; ok {
		t.Error("a LinkedIn row hydrated 5 days ago reads as seen: it would never be re-read")
	}
	if _, ok := set["7002"]; !ok {
		t.Error("a freshly hydrated LinkedIn row reads as unseen")
	}
}
