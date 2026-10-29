//go:build integration

// Integration test for the board-scoped close of a sliceBoardListing provider (whatjobs): a
// keyword board the run finished retires a stored posting it has not listed for the grace window,
// even when that posting's company is one the run never wrote — the leak the company scope left
// to cmd/liveness's 45-day age rule (5,835 of whatjobs-uk's 5,880 postings unseen past 14 days
// on 2026-10-28). A board the run did not crawl, and a posting inside the window, stay open.
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

// keywordSliceSource lists one posting per keyword board, by a company none of the stale seeds
// below share, so only the board scope can reach them.
type keywordSliceSource struct{ provider string }

func (s keywordSliceSource) Provider() string { return s.provider }

func (s keywordSliceSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID:  "1001",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Location:    "London, United Kingdom",
		Description: "<p>A posting the crawl listed this run.</p>",
		URL:         "https://example.com/1001",
	}}, nil
}

func TestSliceBoardListingClosesWhatAFinishedKeywordBoardStoppedListing(t *testing.T) {
	const provider = "whatjobs-uk"
	// Taxonomy, not All: the markers the sweep reads must not depend on this host holding the
	// publisher credential, and the sweep never fetches through the registry.
	registry := sources.Taxonomy()
	if !sources.SliceBoardListingProviders(registry)[provider] {
		t.Fatalf("fixture: %s must carry the sliceBoardListing marker", provider)
	}
	grace := sources.SweepGraceWindows(registry)[provider]

	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	seed := func(board, id, company string, seenAgo time.Duration) int64 {
		t.Helper()
		externalID := externalid.Namespace(board, id)
		j, err := job.New(job.Draft{
			Input: jobderive.Input{
				Source: provider, ExternalID: externalID, Title: "Data Engineer",
				Company: company, Location: "London, United Kingdom",
				Description: "<p>A posting the keyword no longer lists.</p>",
			},
			URL: "https://example.com/" + id,
		})
		if err != nil {
			t.Fatalf("build %s: %v", externalID, err)
		}
		// A throwaway store, so the seeds do not join the crawled companies the run's sweep reads.
		if err := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()).Save(ctx, j); err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: provider, ExternalID: externalID})
		if err != nil {
			t.Fatalf("load %s: %v", externalID, err)
		}
		ageJobForSweepTest(t, pool, row.ID, seenAgo)
		return row.ID
	}
	gone := seed("data engineer", "2002", "Gone Corp", grace+24*time.Hour)
	drifting := seed("data engineer", "3003", "Drift Ltd", grace-24*time.Hour)
	// A keyword board this run never reached — quiet-paused, cooled, or left out by --board.
	unvisited := seed("devops engineer", "4004", "Elsewhere Inc", grace+24*time.Hour)

	crawled := newCrawledSet()
	store := newDBStore(pool, 1, crawled, nil, defaultSeenPolicy())
	runner := pipeline.Runner{Registry: map[string]sources.Source{provider: keywordSliceSource{provider}}, Store: store}
	runStats, err := runner.Run(ctx, []sources.CompanyEntry{
		{Company: "WhatJobs UK", Provider: provider, Board: "data engineer"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if failed := sweep(ctx, q, registry, runStats, crawled, nil, false); failed != 0 {
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
		t.Error("a posting the finished keyword board has not listed for the grace window must close, whatever its company")
	}
	if closed(drifting) {
		t.Error("a posting unseen for less than the grace window was closed — it may only have drifted past the page budget")
	}
	if closed(unvisited) {
		t.Error("a posting on a keyword board this run never crawled was closed — nothing looked at that board")
	}
	listed, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: provider, ExternalID: externalid.Namespace("data engineer", "1001")})
	if err != nil {
		t.Fatalf("load the posting the crawl listed: %v", err)
	}
	if listed.ClosedAt.Valid {
		t.Error("the posting the crawl listed this run must stay open")
	}
}
