package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// maxChronicBoardsPerRun bounds ListChronicBoards's result. A safety rail, not a working limit:
// very few boards should cross the closure window, so more than this is worth a human noticing.
const maxChronicBoardsPerRun = 10000

// closeReport summarizes one pass of either safety net: boards found past the closure window,
// boards skipped as region-ambiguous, and jobs closed (or, without --apply, that would close).
type closeReport struct {
	boardsProcessed        int
	boardsSkippedAmbiguous int
	jobsAffected           int64
}

// boardRef is the (provider, board) identity the region-ambiguity check and log lines need,
// common to a chronic row and an empty-feed row. The two list queries return distinct generated
// types, so the helpers take this.
type boardRef struct{ provider, board, region string }

// String renders a health record the way both passes' logs name one: "provider/board", or just
// the provider for a boardless record, whose empty board would otherwise print a trailing slash.
func (b boardRef) String() string {
	if b.board == "" {
		return b.provider
	}
	if b.region != "" {
		return b.provider + "/" + b.board + " (" + b.region + ")"
	}
	return b.provider + "/" + b.board
}

// isRegionAmbiguous reports whether ref's board name is registered under more than one region
// for its provider (see CountBoardHealthRegions), the hazard ambiguousRegionBoards guards
// against in the ordinary sweep. A boardless provider's record (board == "") is never ambiguous.
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

func logBoardAction(kind string, ref boardRef, n int64, apply bool, applyFlag string) {
	if apply {
		log.Printf("close-chronic-boards: closed %d job(s) for %s board %s", n, kind, ref)
		return
	}
	log.Printf("close-chronic-boards: would close %d job(s) for %s board %s (%s to close)", n, kind, ref, applyFlag)
}

// logCapOverflow reports the boards a pass could not reach because maxChronicBoardsPerRun cut
// the list short; a truncation must never be silent.
func logCapOverflow(kind string, total int64, processed int) {
	if total <= int64(processed) {
		return
	}
	log.Printf("close-chronic-boards: %d more %s board(s) exist beyond the %d this run processed (maxChronicBoardsPerRun cap)",
		total-int64(processed), kind, processed)
}

// closeChronicBoards lists the boards (or boardless providers) with no successful crawl for at
// least closeWindowDays, and for each either closes its open jobs (apply) or counts how many
// would close (dry run), never both in one call. A board with `board == ""` is a boardless
// provider's own health record and routes to the source-scoped close/count, since it stands for
// the provider's whole catalogue.
func closeChronicBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (closeReport, error) {
	ageWindow := pgtype.Interval{Days: closeWindowDays, Valid: true}
	rows, err := q.ListChronicBoards(ctx, db.ListChronicBoardsParams{
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
			logAmbiguousSkip("chronic", ref)
			continue
		}
		n, err := closeOrCountOneBoard(ctx, q, ref, ageWindow, apply)
		if err != nil {
			return report, err
		}
		report.jobsAffected += n
		logBoardAction("chronic", ref, n, apply, "--apply")
	}
	if len(rows) > 0 {
		logCapOverflow("chronic", rows[0].Total, len(rows))
	}
	return report, nil
}

// closeBoardGoneBoards handles only boards whose adapter returned ErrBoardGone on three or more
// consecutive runs spanning closeWindowDays, while another board from the provider succeeded
// in the last 48 hours. It is separate from the generic unreachable pass so the evidence and
// close reason remain explicit, and separately armed by --apply-board-gone.
func closeBoardGoneBoards(ctx context.Context, q *db.Queries, closeWindowDays int32, maxBoards int32, apply bool) (closeReport, error) {
	ageWindow := pgtype.Interval{Days: closeWindowDays, Valid: true}
	rows, err := q.ListBoardGoneBoards(ctx, db.ListBoardGoneBoardsParams{
		AgeWindow: ageWindow,
		MaxBoards: maxBoards,
	})
	if err != nil {
		return closeReport{}, err
	}

	registry := sources.Taxonomy()
	report := closeReport{boardsProcessed: len(rows)}
	for _, r := range rows {
		ref := boardRef{r.Provider, r.Board, r.Region}
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
			logAmbiguousSkip("board-gone", ref)
			continue
		}
		pattern := externalid.BoardPattern(ref.board)
		var n int64
		if apply {
			n, err = q.CloseBoardGoneJobs(ctx, db.CloseBoardGoneJobsParams{
				Source: ref.provider, BoardPattern: pattern, Board: ref.board, Region: ref.region, AgeWindow: ageWindow,
			})
		} else {
			n, err = q.CountBoardGoneJobs(ctx, db.CountBoardGoneJobsParams{
				Source: ref.provider, BoardPattern: pattern, Board: ref.board, Region: ref.region, AgeWindow: ageWindow,
			})
		}
		if err != nil {
			return report, err
		}
		report.jobsAffected += n
		logBoardAction("board-gone", ref, n, apply, "--apply-board-gone")
	}
	if len(rows) > 0 {
		logCapOverflow("board-gone", rows[0].Total, len(rows))
	}
	return report, nil
}

// closeOrCountOneBoard closes (or, without apply, counts) one chronic row's jobs. ageWindow is
// passed to the query, which re-validates board_health's current state: the board can recover
// between the earlier ListChronicBoards read and this call (see CloseChronicBoardJobs).
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
