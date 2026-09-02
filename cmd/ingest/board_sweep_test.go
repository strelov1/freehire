package main

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
)

// The board-scoped close retires a board's stale postings because the BOARD was listed, not
// because the run happened to write something for their company — the only way to reach a
// company whose last posting left a board we still crawl. These tests pin which providers are
// allowed to do that; the pipeline has already decided which of their boards qualify.

func TestBoardSweepTargets(t *testing.T) {
	covered := pipeline.Stats{Ingested: 3, SweepableBoards: []string{"acme", "globex"}}

	for _, tc := range []struct {
		name        string
		stats       pipeline.Stats
		fullCatalog bool
		slicedCrawl bool
		want        []string
	}{
		{
			name:  "an ordinary provider sweeps the boards the run covered",
			stats: covered,
			want:  []string{"acme", "globex"},
		},
		{
			name:  "a provider whose boards all proved nothing sweeps none",
			stats: pipeline.Stats{Ingested: 3},
			want:  nil,
		},
		{
			// The marker means the crawl deliberately reaches only a SLICE of the catalogue, so
			// a posting that merely drifted past the crawl's depth reads as unseen. Closing
			// within such a board would retire live postings and reopen them next run.
			name:        "a provider whose crawl reaches only a slice sweeps none",
			stats:       covered,
			slicedCrawl: true,
			want:        nil,
		},
		{
			// A board listed twice — a repeated entry, or one board id recurring across
			// regional slices — must not produce two identical statements and two log lines.
			name:  "a board listed twice is swept once",
			stats: pipeline.Stats{Ingested: 1, SweepableBoards: []string{"acme", "acme"}},
			want:  []string{"acme"},
		},
		{
			// It already closes by source alone on a clean run, which is strictly broader.
			name:        "a full-catalogue provider sweeps none",
			stats:       covered,
			fullCatalog: true,
			want:        nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := boardSweepTargets(tc.stats, tc.fullCatalog, tc.slicedCrawl)
			if !slices.Equal(got, tc.want) {
				t.Errorf("boardSweepTargets = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBoardSweepTargetsAreDeterministic(t *testing.T) {
	// Boards finish in whatever order their goroutines do, so the sweep sorts them — a
	// deterministic order keeps the per-board log lines comparable between runs.
	stats := pipeline.Stats{Ingested: 1, SweepableBoards: []string{"zulu", "alpha", "mike"}}
	got := boardSweepTargets(stats, false, false)
	if !slices.Equal(got, []string{"alpha", "mike", "zulu"}) {
		t.Errorf("boardSweepTargets = %v, want them sorted", got)
	}
}
