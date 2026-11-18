//go:build integration

// sweep's return value feeds the run's exit code: a close that failed, or left unclosable rows,
// must be counted so a sweep that silently leaves stale postings open does not exit 0.
// Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// seedStaleGreenhouse stores one open greenhouse posting last seen past the grace window and
// returns its id and company slug.
func seedStaleGreenhouse(t *testing.T, ctx context.Context, pool *pgxpool.Pool, externalID string) (int64, string) {
	t.Helper()
	if err := newDBStore(pool, 1, nil, nil, defaultSeenPolicy()).Save(ctx, sweepTestPosting("greenhouse", externalID, "Backend Engineer")); err != nil {
		t.Fatal(err)
	}
	row, err := db.New(pool).GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: externalID})
	if err != nil {
		t.Fatal(err)
	}
	ageJobForSweepTest(t, pool, row.ID, staleAfter+24*time.Hour)
	return row.ID, row.CompanySlug
}

func TestSweep_CountsAFailedCompanyClose(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	_, slug := seedStaleGreenhouse(t, ctx, pool, "acme:1")
	crawled := newCrawledSet()
	crawled.record("greenhouse", slug)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	stats := pipeline.RunStats{"greenhouse": {Ingested: 1}}
	if failed := sweep(cancelled, db.New(pool), sources.Taxonomy(), stats, crawled, nil, false, false); failed != 1 {
		t.Errorf("cancelled sweep failed = %d, want 1", failed)
	}
}

func TestSweep_CountsAFailedBoardScopedClose(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	seedStaleGreenhouse(t, ctx, pool, "acme:1")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	// truncated: only the board scope runs, so a non-zero count can come only from its failure.
	stats := pipeline.RunStats{"greenhouse": {Ingested: 1, QualifyingBoards: []pipeline.BoardRef{{Board: "acme"}}}}
	if failed := sweep(cancelled, db.New(pool), sources.Taxonomy(), stats, newCrawledSet(), nil, true, false); failed == 0 {
		t.Error("cancelled board-scoped sweep failed = 0, want > 0")
	}
}

func TestSweep_CountsUnclosableRows(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	id, slug := seedStaleGreenhouse(t, ctx, pool, "acme:1")
	blockCloseFor(t, pool, id)
	crawled := newCrawledSet()
	crawled.record("greenhouse", slug)

	stats := pipeline.RunStats{"greenhouse": {Ingested: 1}}
	if failed := sweep(ctx, db.New(pool), sources.Taxonomy(), stats, crawled, nil, false, false); failed != 1 {
		t.Errorf("sweep with an unclosable row failed = %d, want 1", failed)
	}
}
