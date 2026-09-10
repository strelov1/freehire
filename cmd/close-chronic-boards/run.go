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

// closeReport summarizes one pass of either safety net: how many boards were found past its
// closure window, how many of those were skipped as region-ambiguous, and how many jobs were
// (or, without --apply, would be) closed across the rest. Shared by both passes because the
// SHAPE of the answer is the same question — what did you find, what did you refuse, what did
// you do — even though the evidence each pass reads is different.
type closeReport struct {
	boardsProcessed        int
	boardsSkippedAmbiguous int
	jobsAffected           int64
}

// boardRef is the (provider, board) identity the region-ambiguity check and the log lines need,
// and the only part of a chronic row and an empty-feed row that is common to both. The two list
// queries return distinct generated row types with no interface between them, so the helpers
// below take this instead of being written once per row type.
type boardRef struct{ provider, board string }

// String renders a health record the way both passes' logs name one: "provider/board", or just
// the provider for a boardless record, whose empty board would otherwise print a trailing slash.
func (b boardRef) String() string {
	if b.board == "" {
		return b.provider
	}
	return b.provider + "/" + b.board
}

// isRegionAmbiguous reports whether ref's board name is registered under more than one region
// for its provider (see CountBoardHealthRegions) — the same hazard the ordinary sweep's
// ambiguousRegionBoards guards against, checked directly against board_health since this worker
// has no crawl-run board list to consult. A boardless provider's own record (board == "") is
// never ambiguous in this sense: it already IS the whole provider, with no board-name collision
// possible.
func isRegionAmbiguous(ctx context.Context, q *db.Queries, ref boardRef) (bool, error) {
	if ref.board == "" {
		return false, nil
	}
	regions, err := q.CountBoardHealthRegions(ctx, db.CountBoardHealthRegionsParams{Provider: ref.provider, Board: ref.board})
	if err != nil {
		return false, err
	}
	return regions > 1, nil
}

// logAmbiguousSkip and logBoardAction are the two passes' shared log lines; kind names which
// safety net is speaking ("chronic", "empty-feed") so one worker's output stays readable when
// both passes have something to say in the same run.
func logAmbiguousSkip(kind string, ref boardRef) {
	log.Printf("close-chronic-boards: skipping %s board %s — its board name is region-ambiguous "+
		"(board_health holds it under more than one region, and jobs.external_id carries no region), "+
		"so a board-scoped close could close a healthy region's jobs alongside this one", kind, ref)
}

func logBoardAction(kind string, ref boardRef, n int64, apply bool) {
	if apply {
		log.Printf("close-chronic-boards: closed %d job(s) for %s board %s", n, kind, ref)
		return
	}
	log.Printf("close-chronic-boards: would close %d job(s) for %s board %s (--apply to close)", n, kind, ref)
}

// logCapOverflow reports the boards a pass could not reach because maxChronicBoardsPerRun cut
// the list short — a truncation that must never be silent, since the whole point of the cap is
// that crossing it is itself worth noticing.
func logCapOverflow(kind string, total int64, processed int) {
	if total <= int64(processed) {
		return
	}
	log.Printf("close-chronic-boards: %d more %s board(s) exist beyond the %d this run processed (maxChronicBoardsPerRun cap)",
		total-int64(processed), kind, processed)
}

// closeChronicBoards lists the boards (or boardless providers) that board_health proves have
// had no successful crawl for at least closeWindowDays, and for each one either closes its
// open jobs (apply) or counts how many would close (dry run) — never both in the same call, so
// a dry run genuinely writes nothing. A board with `board == ""` is a boardless provider's own
// health record (see ingest-board-health spec) and routes to the source-scoped close/count
// instead of the board-scoped one, since it already stands for the provider's whole catalogue.
func closeChronicBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (closeReport, error) {
	ageWindow := pgtype.Interval{Days: closeWindowDays, Valid: true}
	rows, err := q.ListChronicBoards(ctx, db.ListChronicBoardsParams{
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
			logAmbiguousSkip("chronic", ref)
			continue
		}
		n, err := closeOrCountOneBoard(ctx, q, ref, ageWindow, apply)
		if err != nil {
			return report, err
		}
		report.jobsAffected += n
		logBoardAction("chronic", ref, n, apply)
	}
	if len(rows) > 0 {
		logCapOverflow("chronic", rows[0].Total, len(rows))
	}
	return report, nil
}

// closeOrCountOneBoard closes (or, without apply, counts) one chronic row's jobs. ageWindow is
// passed through to the query itself, which re-validates board_health's CURRENT state against
// it before touching anything — see CloseChronicBoardJobs's doc comment for why: the row was
// read by an earlier, separate query (ListChronicBoards), and the board can recover in the gap
// between that read and this call.
func closeOrCountOneBoard(ctx context.Context, q *db.Queries, ref boardRef, ageWindow pgtype.Interval, apply bool) (int64, error) {
	if ref.board == "" {
		if apply {
			return q.CloseChronicProviderJobs(ctx, db.CloseChronicProviderJobsParams{Source: ref.provider, AgeWindow: ageWindow})
		}
		return q.CountChronicProviderJobs(ctx, db.CountChronicProviderJobsParams{Source: ref.provider, AgeWindow: ageWindow})
	}
	pattern := externalid.BoardPattern(ref.board)
	if apply {
		return q.CloseChronicBoardJobs(ctx, db.CloseChronicBoardJobsParams{
			Source: ref.provider, BoardPattern: pattern, Board: ref.board, AgeWindow: ageWindow,
		})
	}
	return q.CountChronicBoardJobs(ctx, db.CountChronicBoardJobsParams{
		Source: ref.provider, BoardPattern: pattern, Board: ref.board, AgeWindow: ageWindow,
	})
}
