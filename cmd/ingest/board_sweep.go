package main

import (
	"context"
	"log"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// boardSweepTargets returns the boards of one provider whose stale postings this run may close,
// sorted for a deterministic sweep order and comparable log lines.
//
// The pipeline has already decided which boards the run PROVED it covered (see
// pipeline.Stats.SweepableBoards). What is decided here is whether the PROVIDER may be swept
// this way at all, and every exclusion is keyed on a marker rather than on a name:
//
//   - slicedCrawl (the adapter declares a wider sweep grace) means the crawl deliberately
//     reaches only a SLICE of the catalogue, so a posting that merely drifted past the crawl's
//     depth reads as unseen. That is precisely when closing within a board is wrong.
//   - selfClosing means the feed's own removals are authoritative and it re-reports only what
//     changed, so "not listed this run" means nothing.
//   - fullCatalog already closes by source alone on a clean run, which is strictly broader than
//     any board scope.
func boardSweepTargets(stats pipeline.Stats, selfClosing, fullCatalog, slicedCrawl bool) []string {
	if selfClosing || fullCatalog || slicedCrawl || len(stats.SweepableBoards) == 0 {
		return nil
	}
	boards := slices.Clone(stats.SweepableBoards)
	slices.Sort(boards)
	return boards
}

// sweepBoards closes each named board's postings unseen past the cutoff, one statement per
// board, and returns how many it closed and how many boards it could not sweep.
//
// One statement per board rather than one for the provider is what makes a corrupted row cost a
// single board instead of the provider's whole sweep — the 2026-08-11 incident, where one
// duplicated jobs_pkey value blocked greenhouse's close on every run. That is also why there is
// no row-by-row fallback here: the blast radius is already one board, and a second code path to
// rescue it would not earn its keep.
func sweepBoards(ctx context.Context, queries *db.Queries, provider string, boards []string,
	cutoff pgtype.Timestamptz) (closed int64, failedBoards int) {
	for _, board := range boards {
		n, err := queries.CloseUnseenJobsForBoard(ctx, db.CloseUnseenJobsForBoardParams{
			Source:       provider,
			BoardPattern: externalid.BoardPattern(board),
			Cutoff:       cutoff,
		})
		if err != nil {
			failedBoards++
			log.Printf("close stale jobs (%s board %q): %v", provider, board, err)
			continue
		}
		if n > 0 {
			// Per board, not per provider: a provider-level number cannot tell "many boards
			// each retiring a few rows" from "one board mass-closing", and that distinction is
			// the whole of what the first fleet cycle after this ships needs to be readable.
			log.Printf("closed %d stale %s jobs on board %q (board was crawled and did not list them)",
				n, provider, board)
		}
		closed += n
	}
	return closed, failedBoards
}
