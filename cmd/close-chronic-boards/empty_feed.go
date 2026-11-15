package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// closeEmptyFeedBoards lists the boards board_health proves are reachable-but-empty for at least
// closeWindowDays, and for each either closes its open jobs (apply) or counts how many would
// close (dry run), never both in one call, so a dry run writes nothing.
//
// Twin of closeChronicBoards for the case it cannot see: a board whose crawls all SUCCEED with
// zero results shows consecutive_failures = 0, so neither the per-run sweep nor the chronic net
// reaches its stale postings (migration 0158).
//
// Kept separate rather than one function with a predicate flag: what each pass proves differs,
// and a flag would put the two safety nets' evidence one boolean apart.
func closeEmptyFeedBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (closeReport, error) {
	ageWindow := pgtype.Interval{Days: closeWindowDays, Valid: true}
	rows, err := q.ListEmptyFeedBoards(ctx, db.ListEmptyFeedBoardsParams{
		AgeWindow: ageWindow,
		MaxBoards: maxBoards,
	})
	if err != nil {
		return closeReport{}, err
	}

	registry := sources.Taxonomy()
	report := closeReport{boardsProcessed: len(rows)}
	for _, r := range rows {
		ref := boardRef{provider: r.Provider, board: r.Board}
		if sources.ProbeCloses(registry[ref.provider]) {
			log.Printf("close-chronic-boards: skipping board %s — requires individual posting evidence", ref)
			continue
		}
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
		logBoardAction("empty-feed", ref, n, apply, "--apply-empty-feed")
	}
	if len(rows) > 0 {
		logCapOverflow("empty-feed", rows[0].Total, len(rows))
	}
	return report, nil
}

// closeOrCountOneEmptyFeedBoard closes (or, without apply, counts) one empty-feed row's jobs. A
// boardless provider's record (board == "") routes to the source-scoped statement, as in
// closeOrCountOneBoard.
//
// ageWindow is passed to the query, which re-validates board_health's current state before
// touching anything: the row was read by an earlier query, and one crawl in the gap makes the
// board live again.
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
