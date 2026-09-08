package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// maxChronicBoardsPerRun bounds ListChronicBoards's result for this worker, mirroring the
// per-run report's cap (cmd/ingest's chronicBoardsCap) but far more generous: this is a daily
// one-off, not a hot per-run log line, and the whole point of the chronic mechanism is that
// very few boards should ever cross the closure window. A real run naming more than this many
// would itself be worth a human noticing, so the cap stays a safety rail, not a working limit.
const maxChronicBoardsPerRun = 10000

// chronicBoardsReport summarizes one pass: how many chronic boards were found past the closure
// window, how many of those were skipped as region-ambiguous (see closeOrCountOneBoard), and
// how many jobs were (or, without --apply, would be) closed across the rest.
type chronicBoardsReport struct {
	boardsProcessed        int
	boardsSkippedAmbiguous int
	jobsAffected           int64
}

// closeChronicBoards lists the boards (or boardless providers) that board_health proves have
// had no successful crawl for at least closeWindowDays, and for each one either closes its
// open jobs (apply) or counts how many would close (dry run) — never both in the same call, so
// a dry run genuinely writes nothing. A board with `board == ""` is a boardless provider's own
// health record (see ingest-board-health spec) and routes to the source-scoped close/count
// instead of the board-scoped one, since it already stands for the provider's whole catalogue.
func closeChronicBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (chronicBoardsReport, error) {
	rows, err := q.ListChronicBoards(ctx, db.ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: closeWindowDays, Valid: true},
		MaxBoards: maxBoards,
	})
	if err != nil {
		return chronicBoardsReport{}, err
	}

	report := chronicBoardsReport{boardsProcessed: len(rows)}
	for _, r := range rows {
		ambiguous, err := isRegionAmbiguous(ctx, q, r)
		if err != nil {
			return report, err
		}
		if ambiguous {
			report.boardsSkippedAmbiguous++
			log.Printf("close-chronic-boards: skipping %s/%s — its board name is region-ambiguous "+
				"(board_health holds it under more than one region, and jobs.external_id carries no "+
				"region), so a board-scoped close could close a healthy region's jobs alongside this one",
				r.Provider, r.Board)
			continue
		}
		n, err := closeOrCountOneBoard(ctx, q, r, apply)
		if err != nil {
			return report, err
		}
		report.jobsAffected += n
		logChronicBoardAction(r, n, apply)
	}
	if len(rows) > 0 && rows[0].Total > int64(len(rows)) {
		log.Printf("close-chronic-boards: %d more chronic board(s) exist beyond the %d this run processed (maxChronicBoardsPerRun cap)",
			rows[0].Total-int64(len(rows)), len(rows))
	}
	return report, nil
}

// isRegionAmbiguous reports whether r's board name is registered under more than one region
// for its provider (see CountBoardHealthRegions) — the same hazard the ordinary sweep's
// ambiguousRegionBoards guards against, checked directly against board_health since this
// worker has no crawl-run board list to consult. A boardless provider's own record (board ==
// "") is never ambiguous in this sense: it already IS the whole provider, with no board-name
// collision possible.
func isRegionAmbiguous(ctx context.Context, q *db.Queries, r db.ListChronicBoardsRow) (bool, error) {
	if r.Board == "" {
		return false, nil
	}
	regions, err := q.CountBoardHealthRegions(ctx, db.CountBoardHealthRegionsParams{Provider: r.Provider, Board: r.Board})
	if err != nil {
		return false, err
	}
	return regions > 1, nil
}

func closeOrCountOneBoard(ctx context.Context, q *db.Queries, r db.ListChronicBoardsRow, apply bool) (int64, error) {
	if r.Board == "" {
		if apply {
			return q.CloseChronicProviderJobs(ctx, r.Provider)
		}
		return q.CountChronicProviderJobs(ctx, r.Provider)
	}
	pattern := externalid.BoardPattern(r.Board)
	if apply {
		return q.CloseChronicBoardJobs(ctx, db.CloseChronicBoardJobsParams{Source: r.Provider, BoardPattern: pattern})
	}
	return q.CountChronicBoardJobs(ctx, db.CountChronicBoardJobsParams{Source: r.Provider, BoardPattern: pattern})
}

func logChronicBoardAction(r db.ListChronicBoardsRow, n int64, apply bool) {
	id := r.Provider
	if r.Board != "" {
		id += "/" + r.Board
	}
	if apply {
		log.Printf("close-chronic-boards: closed %d job(s) for chronic board %s", n, id)
		return
	}
	log.Printf("close-chronic-boards: would close %d job(s) for chronic board %s (--apply to close)", n, id)
}
