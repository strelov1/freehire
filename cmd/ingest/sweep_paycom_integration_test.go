//go:build integration

// Integration test for paycom's board-scoped close. Paycom runs routinely meet their crawl
// deadline, and a truncated run closes through the board scope only — which paycom, lacking
// fullBoardListing, was never allowed. A posting the employer took down therefore stayed open
// until a crawl of its board happened to land in a clean run (Brown Sims PC on 2026-10-29: a
// posting absent from the listing since 10-17, validThrough 10-22, still open). With the marker,
// a board the truncated run FINISHED closes what it stopped listing; a board it did not reach
// keeps its rows.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// onePostingSource lists one live posting on every board it is asked for.
type onePostingSource struct{ provider string }

func (s onePostingSource) Provider() string { return s.provider }

func (s onePostingSource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID:  "103900",
		Title:       "Paralegal",
		Company:     e.Company,
		Location:    "Houston, TX",
		Description: "<p>A posting the board still lists.</p>",
		URL:         "https://example.com/103900",
	}}, nil
}

func TestTruncatedPaycomRunClosesWhatAFinishedBoardStoppedListing(t *testing.T) {
	const provider = "paycom"
	registry := sources.Taxonomy()
	if !sources.FullBoardListingProviders(registry)[provider] {
		t.Fatalf("fixture: %s must carry the fullBoardListing marker", provider)
	}
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	seed := func(board, id string) int64 {
		t.Helper()
		externalID := externalid.Namespace(board, id)
		j, err := job.New(job.Draft{
			Input: jobderive.Input{
				Source: provider, ExternalID: externalID, Title: "Legal Assistant",
				Company: "Brown Sims PC", Location: "Houston, TX",
				Description: "<p>A posting the employer has taken down.</p>",
			},
			URL: "https://example.com/" + id,
		})
		if err != nil {
			t.Fatalf("build %s: %v", externalID, err)
		}
		if err := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()).Save(ctx, j); err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: provider, ExternalID: externalID})
		if err != nil {
			t.Fatalf("load %s: %v", externalID, err)
		}
		ageJobForSweepTest(t, pool, row.ID, staleAfter+5*24*time.Hour)
		return row.ID
	}
	gone := seed("243fdad5", "103855")
	unreached := seed("4b1ca72f", "103855")

	crawled := newCrawledSet()
	store := newDBStore(pool, 1, crawled, nil, defaultSeenPolicy())
	runner := pipeline.Runner{Registry: map[string]sources.Source{provider: onePostingSource{provider}}, Store: store}
	runStats, err := runner.Run(ctx, []sources.CompanyEntry{{Company: "Brown Sims PC", Provider: provider, Board: "243fdad5"}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// truncated: the run's deadline stopped the crawl before it reached the second board.
	if failed := sweep(ctx, q, registry, runStats, crawled, nil, true); failed != 0 {
		t.Fatalf("sweep reported %d failures", failed)
	}

	closed := func(id int64) bool {
		t.Helper()
		row, err := q.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		return row.ClosedAt.Valid
	}
	if !closed(gone) {
		t.Error("a posting the finished board stopped listing must close, even in a truncated run")
	}
	if closed(unreached) {
		t.Error("a posting on a board the truncated run never reached was closed")
	}
}
