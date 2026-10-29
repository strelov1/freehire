//go:build integration

// Integration test for the region-scoped board close. A jobstreet board id (an ICT
// subclassification, "6290") exists in six markets and external_id carries no market, so the
// board scope used to refuse every jobstreet board as region-ambiguous and only the company
// scope remained: a company that left the feed stranded its rows open (2,794 jobstreet rows
// unseen past 48h on 2026-10-29, 106 of them under a company with a fresh posting). The close
// now narrows each finished (board, region) to the market's posting host, so one market's crawl
// closes what it stopped listing there and never touches the same board in another market.
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

// marketSource lists one live posting per board on the market's own host, and scopes its boards
// by region the way the real jobstreet adapter does.
type marketSource struct{ provider string }

func (s marketSource) Provider() string { return s.provider }

func (s marketSource) BoardURLPrefix(region string) (string, bool) {
	return sources.BoardURLPrefix(sources.Taxonomy()["jobstreet"], region)
}

func (s marketSource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	prefix, _ := s.BoardURLPrefix(e.Region)
	return []sources.Job{{
		ExternalID:  "1001",
		Title:       "Backend Engineer",
		Company:     "Acme Pte Ltd",
		Location:    "Singapore",
		Description: "<p>A posting the market still lists.</p>",
		URL:         prefix + "1001",
	}}, nil
}

func TestRegionScopedBoardClosesOnlyTheMarketTheRunFinished(t *testing.T) {
	const provider = "jobstreet"
	registry := sources.Taxonomy()
	if !sources.FullBoardListingProviders(registry)[provider] || !sources.ScopesBoardsByRegion(registry[provider]) {
		t.Fatalf("fixture: %s must carry fullBoardListing and scope its boards by region", provider)
	}
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	seed := func(region, id, company string) int64 {
		t.Helper()
		prefix, _ := sources.BoardURLPrefix(registry[provider], region)
		externalID := externalid.Namespace("6290", id)
		j, err := job.New(job.Draft{
			Input: jobderive.Input{
				Source: provider, ExternalID: externalID, Title: "Data Engineer",
				Company: company, Location: "Singapore",
				Description: "<p>A posting the market no longer lists.</p>",
			},
			URL: prefix + id,
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
		ageJobForSweepTest(t, pool, row.ID, staleAfter+24*time.Hour)
		return row.ID
	}
	// Both under companies the run never writes: only the board scope can reach them.
	goneSG := seed("sg", "2002", "Left The Feed Pte Ltd")
	otherMarket := seed("my", "3003", "Left The Feed Sdn Bhd")

	entries := []sources.CompanyEntry{
		{Company: "JobStreet SG", Provider: provider, Board: "6290", Region: "sg"},
		{Company: "JobStreet MY", Provider: provider, Board: "6290", Region: "my"},
	}
	// The full catalogue holds the board in both markets; the run crawls only SG (MY is
	// quiet-paused, cooled, or in another shard).
	crossShard := pipeline.AmbiguousBoardNames(entries, registry)
	if len(crossShard) != 0 {
		t.Fatalf("AmbiguousBoardNames = %v, want none for a region-scoped provider", crossShard)
	}
	crawled := newCrawledSet()
	store := newDBStore(pool, 1, crawled, nil, defaultSeenPolicy())
	runner := pipeline.Runner{Registry: map[string]sources.Source{provider: marketSource{provider}}, Store: store}
	runStats, err := runner.Run(ctx, entries[:1])
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if failed := sweep(ctx, q, registry, runStats, crawled, crossShard, false); failed != 0 {
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
	if !closed(goneSG) {
		t.Error("a posting the finished SG board stopped listing must close, whatever its company")
	}
	if closed(otherMarket) {
		t.Error("the same board id in a market this run never crawled was closed")
	}
}
