package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

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
// The two passes are kept as separate functions rather than one parameterised by predicate: they
// share the shape of the walk (list, refuse the ambiguous, close or count, report) and that part
// IS shared below, but what each one proves is different, and a single function taking a "which
// net am I" flag would put the two safety nets' evidence one boolean apart from each other.
func closeEmptyFeedBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (closeReport, error) {
	ageWindow := pgtype.Interval{Days: closeWindowDays, Valid: true}
	rows, err := q.ListEmptyFeedBoards(ctx, db.ListEmptyFeedBoardsParams{
		AgeWindow: ageWindow,
		MaxBoards: maxBoards,
	})
	if err != nil {
		return closeReport{}, err
	}

	report := closeReport{boardsProcessed: len(rows)}
	for _, r := range rows {
		ref := boardRef{r.Provider, r.Board}
		ambiguous, err := isRegionAmbiguous(ctx, q, ref)
		if err != nil {
			return report, err
		}
		if ambiguous {
			report.boardsSkippedAmbiguous++
			logAmbiguousSkip("empty-feed", ref)
			continue
		}
		n, err := closeOrCountOneEmptyFeedBoard(ctx, q, ref, ageWindow, apply)
		if err != nil {
			return report, err
		}
		report.jobsAffected += n
		logBoardAction("empty-feed", ref, n, apply)
	}
	if len(rows) > 0 {
		logCapOverflow("empty-feed", rows[0].Total, len(rows))
	}
	return report, nil
}

// closeOrCountOneEmptyFeedBoard closes (or, without apply, counts) one empty-feed row's jobs. A
// boardless provider's record (board == "") routes to the source-scoped statement, exactly as in
// closeOrCountOneBoard: such a record already stands for the provider's whole crawl.
//
// ageWindow is passed through to the query itself, which re-validates board_health's CURRENT
// state against it before touching anything — see CloseEmptyFeedBoardJobs's doc comment: the row
// was read by an earlier, separate query, and a single crawl that reaches one posting in the gap
// stamps last_yield_at and makes the board live again.
func closeOrCountOneEmptyFeedBoard(ctx context.Context, q *db.Queries, ref boardRef, ageWindow pgtype.Interval, apply bool) (int64, error) {
	if ref.board == "" {
		if apply {
			return q.CloseEmptyFeedProviderJobs(ctx, db.CloseEmptyFeedProviderJobsParams{Source: ref.provider, AgeWindow: ageWindow})
		}
		return q.CountEmptyFeedProviderJobs(ctx, db.CountEmptyFeedProviderJobsParams{Source: ref.provider, AgeWindow: ageWindow})
	}
	pattern := externalid.BoardPattern(ref.board)
	if apply {
		return q.CloseEmptyFeedBoardJobs(ctx, db.CloseEmptyFeedBoardJobsParams{
			Source: ref.provider, BoardPattern: pattern, Board: ref.board, AgeWindow: ageWindow,
		})
	}
	return q.CountEmptyFeedBoardJobs(ctx, db.CountEmptyFeedBoardJobsParams{
		Source: ref.provider, BoardPattern: pattern, Board: ref.board, AgeWindow: ageWindow,
	})
}
