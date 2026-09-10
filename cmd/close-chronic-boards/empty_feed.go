package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// emptyFeedReport summarizes one empty-feed pass, mirroring chronicBoardsReport field for field
// so the two windows read the same way in the run's log. Kept a separate type rather than shared:
// the two passes answer different questions about different columns, and a shared struct would
// invite a shared function whose only body is an if.
type emptyFeedReport struct {
	boardsProcessed        int
	boardsSkippedAmbiguous int
	jobsAffected           int64
}

// closeEmptyFeedBoards lists the boards board_health proves are reachable-but-empty for at least
// closeWindowDays, and for each one either closes its open jobs (apply) or counts how many would
// close (dry run) — never both in the same call, so a dry run genuinely writes nothing.
//
// This is the twin of closeChronicBoards, for the failure mode that one structurally cannot see:
// there, a board whose crawls all FAIL; here, a board whose crawls all SUCCEED and return
// nothing. The distinction is not academic — it decided the WhatJobs incident of 2026-09-10,
// where a market's publisher account was emptied, every crawl answered 200 with zero results,
// board_health showed a green board with consecutive_failures = 0, and a month of stale postings
// stayed open because neither the per-run sweep nor the chronic net could reach them (see
// migration 0158).
//
// A boardless provider's record (board == "") routes to the source-scoped close/count, exactly
// as in closeChronicBoards: such a record already stands for the provider's whole crawl.
func closeEmptyFeedBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (emptyFeedReport, error) {
	ageWindow := pgtype.Interval{Days: closeWindowDays, Valid: true}
	rows, err := q.ListEmptyFeedBoards(ctx, db.ListEmptyFeedBoardsParams{
		AgeWindow: ageWindow,
		MaxBoards: maxBoards,
	})
	if err != nil {
		return emptyFeedReport{}, err
	}

	report := emptyFeedReport{boardsProcessed: len(rows)}
	for _, r := range rows {
		ambiguous, err := isEmptyFeedRegionAmbiguous(ctx, q, r)
		if err != nil {
			return report, err
		}
		if ambiguous {
			report.boardsSkippedAmbiguous++
			log.Printf("close-chronic-boards: skipping empty-feed %s/%s — its board name is region-ambiguous "+
				"(board_health holds it under more than one region, and jobs.external_id carries no "+
				"region), so a board-scoped close could close a healthy region's jobs alongside this one",
				r.Provider, r.Board)
			continue
		}
		n, err := closeOrCountOneEmptyFeedBoard(ctx, q, r, ageWindow, apply)
		if err != nil {
			return report, err
		}
		report.jobsAffected += n
		logEmptyFeedAction(r, n, apply)
	}
	if len(rows) > 0 && rows[0].Total > int64(len(rows)) {
		log.Printf("close-chronic-boards: %d more empty-feed board(s) exist beyond the %d this run processed (maxChronicBoardsPerRun cap)",
			rows[0].Total-int64(len(rows)), len(rows))
	}
	return report, nil
}

// isEmptyFeedRegionAmbiguous is isRegionAmbiguous for an empty-feed row. Same check, same
// reasoning (see that function); separate only because the two list queries return distinct
// generated row types and Go has no structural typing over them.
func isEmptyFeedRegionAmbiguous(ctx context.Context, q *db.Queries, r db.ListEmptyFeedBoardsRow) (bool, error) {
	if r.Board == "" {
		return false, nil
	}
	regions, err := q.CountBoardHealthRegions(ctx, db.CountBoardHealthRegionsParams{Provider: r.Provider, Board: r.Board})
	if err != nil {
		return false, err
	}
	return regions > 1, nil
}

// closeOrCountOneEmptyFeedBoard closes (or, without apply, counts) one empty-feed row's jobs.
// ageWindow is passed through to the query itself, which re-validates board_health's CURRENT
// state against it before touching anything — see CloseEmptyFeedBoardJobs's doc comment: the row
// was read by an earlier, separate query, and a single crawl that reaches one posting in the gap
// stamps last_yield_at and makes the board live again.
func closeOrCountOneEmptyFeedBoard(ctx context.Context, q *db.Queries, r db.ListEmptyFeedBoardsRow, ageWindow pgtype.Interval, apply bool) (int64, error) {
	if r.Board == "" {
		if apply {
			return q.CloseEmptyFeedProviderJobs(ctx, db.CloseEmptyFeedProviderJobsParams{Source: r.Provider, AgeWindow: ageWindow})
		}
		return q.CountEmptyFeedProviderJobs(ctx, db.CountEmptyFeedProviderJobsParams{Source: r.Provider, AgeWindow: ageWindow})
	}
	pattern := externalid.BoardPattern(r.Board)
	if apply {
		return q.CloseEmptyFeedBoardJobs(ctx, db.CloseEmptyFeedBoardJobsParams{
			Source: r.Provider, BoardPattern: pattern, Board: r.Board, AgeWindow: ageWindow,
		})
	}
	return q.CountEmptyFeedBoardJobs(ctx, db.CountEmptyFeedBoardJobsParams{
		Source: r.Provider, BoardPattern: pattern, Board: r.Board, AgeWindow: ageWindow,
	})
}

func logEmptyFeedAction(r db.ListEmptyFeedBoardsRow, n int64, apply bool) {
	id := r.Provider
	if r.Board != "" {
		id += "/" + r.Board
	}
	if apply {
		log.Printf("close-chronic-boards: closed %d job(s) for empty-feed board %s", n, id)
		return
	}
	log.Printf("close-chronic-boards: would close %d job(s) for empty-feed board %s (--apply to close)", n, id)
}
