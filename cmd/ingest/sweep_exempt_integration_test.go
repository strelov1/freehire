//go:build integration

// A self-closing provider (jobtech, linkedin) re-reports only changed ads, so the unseen sweep
// must skip it: dropping the exemption would close every untouched open posting.
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestSweep_SkipsSelfClosingProviders(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	registry := sources.Taxonomy()

	if err := newDBStore(pool, 1, nil, nil, defaultSeenPolicy()).Save(ctx, sweepTestPosting("jobtech", "1001", "Backend Engineer")); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "jobtech", ExternalID: "1001"})
	if err != nil {
		t.Fatal(err)
	}
	ageJobForSweepTest(t, pool, row.ID, staleAfter+24*time.Hour)

	crawled := newCrawledSet()
	crawled.record("jobtech", row.CompanySlug)
	stats := pipeline.RunStats{"jobtech": {Ingested: 1}}
	if failed := sweep(ctx, q, registry, stats, crawled, nil, false, false); failed != 0 {
		t.Fatalf("sweep failed = %d, want 0", failed)
	}
	after, err := q.GetJob(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ClosedAt.Valid {
		t.Errorf("untouched open jobtech ad was closed (reason %q); self-closing providers must be exempt", after.ClosedReason)
	}
}
