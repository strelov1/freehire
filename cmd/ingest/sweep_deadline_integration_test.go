//go:build integration

// Integration test for the crawl deadline (runBudget): a crawl stopped short of its unit's
// timeout must still sweep, on a context its own cancellation cannot reach, and only within the
// boards it read to completion. Until 2026-10-23 a run that outlived its timeout swept nothing
// at all — greenhouse reached its sweep in 0 of 33 runs — so this is the path that runs every
// time a large provider is slower than its unit allows.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
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

// slowBoardSource lists one posting on each board. Its "fast" board answers at once; every other
// board is held until the crawl's context ends and then handed back WITHOUT an error — the
// adapter that swallows a cancellation, which leaves no Failed for the sweep to notice.
type slowBoardSource struct{ provider string }

func (s slowBoardSource) Provider() string { return s.provider }

func (s slowBoardSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	if e.Board != "fast" {
		<-ctx.Done()
	}
	return []sources.Job{{
		ExternalID:  "listed",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Location:    "Remote",
		Description: "<p>A posting the crawl listed this run.</p>",
		URL:         "https://example.com/" + e.Board + "/listed",
	}}, nil
}

func TestCrawlDeadlineSweepsOnlyTheBoardsItFinished(t *testing.T) {
	const provider = "greenhouse"
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	crawled := newCrawledSet()
	store := newDBStore(pool, 1, crawled, nil, defaultSeenPolicy())

	// The sweep's gates come from the real registry: the board scope applies only to a provider
	// registered as listing a board to completion, which greenhouse is.
	registry := sources.All(sources.NewClient())
	if !sources.FullBoardListingProviders(registry)[provider] {
		t.Fatalf("fixture: %s must carry the fullBoardListing marker for the board scope to apply", provider)
	}

	// Two stale postings the crawl will NOT list, one per board, both under the same company —
	// the shape that makes the company scope unsafe on a truncated run.
	seed := func(externalID string) int64 {
		t.Helper()
		if err := store.Save(ctx, sweepTestPosting(provider, externalID, "Platform Engineer")); err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: provider, ExternalID: externalID})
		if err != nil {
			t.Fatalf("load %s: %v", externalID, err)
		}
		ageJobForSweepTest(t, pool, row.ID, 72*time.Hour)
		return row.ID
	}
	goneFromFinished := seed("fast:gone")
	staleOnInterrupted := seed("slow:unreached")

	// The crawl, bounded by a deadline that lands while the slow board is in flight. The parent
	// is then cancelled as well — the signal context a SIGTERM ends — so the sweep below has to
	// survive both of the things that end a crawl.
	parent, cancelParent := context.WithCancel(ctx)
	crawlCtx, cancelCrawl := context.WithTimeout(parent, 300*time.Millisecond)
	runner := pipeline.Runner{Registry: map[string]sources.Source{provider: slowBoardSource{provider}}, Store: store}
	runStats, crawlErr := runner.Run(crawlCtx, []sources.CompanyEntry{
		{Company: "Acme", Provider: provider, Board: "fast"},
		{Company: "Acme", Provider: provider, Board: "slow"},
	})
	cancelCrawl()
	if crawlErr == nil {
		t.Fatal("fixture: the crawl should have hit its deadline")
	}
	cancelParent()

	budget := runBudget{timeout: time.Hour, reserve: 10 * time.Minute}
	sweepCtx, cancelSweep := budget.sweepContext(parent, time.Now())
	defer cancelSweep()
	if failed := sweep(sweepCtx, q, registry, runStats, crawled, nil, true); failed != 0 {
		t.Fatalf("sweep reported %d failures — a context cancelled with the crawl must not reach it", failed)
	}

	closed := func(id int64) bool {
		t.Helper()
		row, err := q.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		return row.ClosedAt.Valid
	}
	if !closed(goneFromFinished) {
		t.Error("a stale posting on a board the crawl finished must be closed — the deadline exists so this still happens")
	}
	if closed(staleOnInterrupted) {
		t.Error("a stale posting on a board the deadline cut short was closed — the crawl never finished reading that board, " +
			"and the company it shares with a finished board must not stand in for it")
	}
	listed, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: provider, ExternalID: "fast:listed"})
	if err != nil {
		t.Fatalf("load the posting the crawl listed: %v", err)
	}
	if listed.ClosedAt.Valid {
		t.Error("the posting the crawl listed this run must stay open")
	}
}
