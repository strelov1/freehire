//go:build integration

// Integration test for the board-scoped close of an EMPTIED board: an employer that took every
// posting down leaves a board whose finished crawl lists nothing. Before, such a board never
// qualified (boardReachedPostings) and its company never re-entered the crawled set, so its last
// postings stayed open until the empty-feed net closed them 30 days later — greenhouse skedda,
// ashby umbrel and ashby gruntwork on 2026-10-29. An adapter that answers an emptied board
// distinguishably from a failed fetch (sources.emptyBoardListing) now closes it on the ordinary
// window; one that cannot (smartrecruiters answers an unknown company empty) still does not.
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

// emptiedBoardSource lists nothing on its "emptied" board and one posting on every other, so the
// provider's run ingests something (shouldSweep) while one of its boards comes back empty.
type emptiedBoardSource struct{ provider string }

func (s emptiedBoardSource) Provider() string { return s.provider }

func (s emptiedBoardSource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	if e.Board == "emptied" {
		return []sources.Job{}, nil
	}
	return []sources.Job{{
		ExternalID:  "1001",
		Title:       "Backend Engineer",
		Company:     e.Company,
		Location:    "Berlin, Germany",
		Description: "<p>A posting the crawl listed this run.</p>",
		URL:         "https://example.com/1001",
	}}, nil
}

func TestEmptiedBoardClosesItsPostingsOnlyForAnAdapterThatProvesEmpty(t *testing.T) {
	registry := sources.Taxonomy()
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	cases := []struct {
		provider  string
		wantClose bool
	}{
		{"greenhouse", true},
		{"smartrecruiters", false},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			if got := sources.EmptyBoardListingProviders(registry)[tc.provider]; got != tc.wantClose {
				t.Fatalf("fixture: %s emptyBoardListing = %v, want %v", tc.provider, got, tc.wantClose)
			}
			seed := func(board, id string, seenAgo time.Duration) int64 {
				t.Helper()
				externalID := externalid.Namespace(board, id)
				j, err := job.New(job.Draft{
					Input: jobderive.Input{
						Source: tc.provider, ExternalID: externalID, Title: "Data Engineer",
						Company: "Skedda", Location: "Berlin, Germany",
						Description: "<p>A posting the employer has since taken down.</p>",
					},
					URL: "https://example.com/" + id,
				})
				if err != nil {
					t.Fatalf("build %s: %v", externalID, err)
				}
				// A throwaway store, so the seeds do not join the crawled companies the sweep reads.
				if err := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()).Save(ctx, j); err != nil {
					t.Fatalf("seed %s: %v", externalID, err)
				}
				row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: tc.provider, ExternalID: externalID})
				if err != nil {
					t.Fatalf("load %s: %v", externalID, err)
				}
				ageJobForSweepTest(t, pool, row.ID, seenAgo)
				return row.ID
			}
			gone := seed("emptied", "2002", staleAfter+24*time.Hour)
			recent := seed("emptied", "3003", staleAfter-24*time.Hour)

			crawled := newCrawledSet()
			store := newDBStore(pool, 1, crawled, nil, defaultSeenPolicy())
			runner := pipeline.Runner{Registry: map[string]sources.Source{tc.provider: emptiedBoardSource{tc.provider}}, Store: store}
			runStats, err := runner.Run(ctx, []sources.CompanyEntry{
				{Company: "Skedda", Provider: tc.provider, Board: "emptied"},
				{Company: "Busy Co", Provider: tc.provider, Board: "busy"},
			})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if failed := sweep(ctx, q, registry, runStats, crawled, nil, false); failed != 0 {
				t.Fatalf("sweep reported %d failures", failed)
			}

			closed := func(id int64) (bool, string) {
				t.Helper()
				row, err := q.GetJob(ctx, id)
				if err != nil {
					t.Fatalf("get %d: %v", id, err)
				}
				return row.ClosedAt.Valid, row.ClosedReason
			}
			if got, reason := closed(gone); got != tc.wantClose {
				t.Errorf("stale posting on the emptied board closed = %v (%q), want %v", got, reason, tc.wantClose)
			} else if got && reason != "unseen" {
				t.Errorf("closed_reason = %q, want unseen — the ordinary board-scoped close", reason)
			}
			if got, _ := closed(recent); got {
				t.Error("a posting on the emptied board seen inside the window was closed")
			}
		})
	}
}
