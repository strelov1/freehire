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
//   - fullCatalog closes by source alone on a clean run, which is strictly broader than any
//     board scope; on a run with a failed board it falls back to the company scope instead, and
//     the board scope stays out of both cases rather than filling the gap. Both such adapters
//     are boardless today, so the exclusion is belt-and-braces — kept because "boardless" is a
//     property of their board files, not of the marker.
//
// A self-closing provider needs no exclusion here: the caller skips it before the sweep runs
// at all. Taking it as a parameter would be a branch no input can reach.
func boardSweepTargets(stats pipeline.Stats, fullCatalog, slicedCrawl bool) []string {
	if fullCatalog || slicedCrawl || len(stats.SweepableBoards) == 0 {
		return nil
	}
	boards := slices.Clone(stats.SweepableBoards)
	slices.Sort(boards)
	// One board can legitimately be listed twice — a board file may repeat an entry, and a
	// provider whose board id recurs across regional slices has one entry per region. Two
	// identical statements would close nothing extra but would double the board's log line and
	// the "across N boards" count, which is the one number the first fleet cycle is read by.
	return slices.Compact(boards)
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
	cutoff pgtype.Timestamptz) (closed int64, boardsClosed, failedBoards int) {
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
			boardsClosed++
			// Per board, not per provider: a provider-level number cannot tell "many boards
			// each retiring a few rows" from "one board mass-closing", and that distinction is
			// the whole of what the first fleet cycle after this ships needs to be readable.
			log.Printf("closed %d stale %s jobs on board %q (board was crawled and did not list them)",
				n, provider, board)
		}
		closed += n
	}
	return closed, boardsClosed, failedBoards
}
