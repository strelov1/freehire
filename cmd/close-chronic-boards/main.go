// Command close-chronic-boards is the safety net for a board (or boardless provider) that has
// been unreachable for a long time, not merely cooling down.
//
// The per-run unseen sweep (cmd/ingest) never closes a board's jobs unless that run proved it
// covered the board, so a permanently gone board keeps stale jobs open forever. Once
// board_health.ListChronicBoards proves no successful crawl for the closure window (default 60
// days, CHRONIC_BOARD_CLOSE_WINDOW_DAYS overrides), its open jobs close with their own
// 'board_unreachable' reason, never 'unseen'.
//
// Dry-run by default; --apply writes. Ship it dry-run on its timer until a human has read at
// least one full closure window's reports.
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// chronicBoardCloseWindowDaysDefault is longer than the per-run report's chronic window
// (cmd/ingest's chronicBoardWindowDaysDefault, 30 days), leaving a curator the gap to retire or
// fix a board before this worker closes it.
const chronicBoardCloseWindowDaysDefault int32 = 60

// emptyFeedCloseWindowDaysDefault is shorter than the unreachable window: an unreachable board
// waits out our side (blocked crawler, rotated URL), while an empty feed has already answered,
// successfully, on every run. 30 days guards against a seasonal or paused board.
const emptyFeedCloseWindowDaysDefault int32 = 30

// Board-gone needs repeated explicit platform evidence, so one week is enough separation from
// transient 404s while materially faster than the generic 60-day unreachable safety net.
const boardGoneCloseWindowDaysDefault int32 = 7

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually close chronically UNREACHABLE boards; without it that pass only reports")
	// A separate switch rather than letting --apply arm both passes: the unit file tells an operator
	// to add --apply after reading the unreachable pass's reports, and that must not silently arm a
	// pass whose reports nobody reviewed.
	applyEmptyFeed := flag.Bool("apply-empty-feed", false, "actually close reachable-but-EMPTY boards; without it that pass only reports")
	applyBoardGone := flag.Bool("apply-board-gone", false, "actually close boards with repeated explicit board-gone evidence; without it that pass only reports")
	flag.Bool("dry-run", false, "no-op: reporting is the default, --apply flags are what close")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	days, err := worker.EnvInt32("CHRONIC_BOARD_CLOSE_WINDOW_DAYS", chronicBoardCloseWindowDaysDefault)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}

	emptyDays, err := worker.EnvInt32("EMPTY_FEED_CLOSE_WINDOW_DAYS", emptyFeedCloseWindowDaysDefault)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}
	boardGoneDays, err := worker.EnvInt32("BOARD_GONE_CLOSE_WINDOW_DAYS", boardGoneCloseWindowDaysDefault)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}
	if boardGoneDays < 1 {
		log.Printf("close-chronic-boards: BOARD_GONE_CLOSE_WINDOW_DAYS must be positive")
		return 1
	}

	q := db.New(pool)

	gone, err := closeBoardGoneBoards(ctx, q, boardGoneDays, maxChronicBoardsPerRun, *applyBoardGone)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}
	goneVerb := "would close"
	if *applyBoardGone {
		goneVerb = "closed"
	}
	log.Printf("close-chronic-boards: %d confirmed-gone board(s) (%d skipped as region-ambiguous), %s %d job(s) total",
		gone.boardsProcessed, gone.boardsSkippedAmbiguous, goneVerb, gone.jobsAffected)

	report, err := closeChronicBoards(ctx, q, days, maxChronicBoardsPerRun, *apply)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}

	verb := "would close"
	if *apply {
		verb = "closed"
	}
	log.Printf("close-chronic-boards: %d chronic board(s) (%d skipped as region-ambiguous), %s %d job(s) total",
		report.boardsProcessed, report.boardsSkippedAmbiguous, verb, report.jobsAffected)

	// The empty-feed pass runs after the unreachable one, never instead of it. The two select
	// disjoint sets (this one needs a recent success, a chronic board has none), and the close
	// statements re-validate anyway.
	empty, err := closeEmptyFeedBoards(ctx, q, emptyDays, maxChronicBoardsPerRun, *applyEmptyFeed)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}
	emptyVerb := "would close"
	if *applyEmptyFeed {
		emptyVerb = "closed"
	}
	log.Printf("close-chronic-boards: %d empty-feed board(s) (%d skipped as region-ambiguous), %s %d job(s) total",
		empty.boardsProcessed, empty.boardsSkippedAmbiguous, emptyVerb, empty.jobsAffected)

	// Publish the verdict where it outlives the journal. Best-effort and last: a failed metrics
	// write must not turn a successful run red.
	publishMetrics(report, empty, gone, *apply, *applyEmptyFeed, *applyBoardGone)
	return 0
}

// publishMetrics writes the run's verdict to the node_exporter textfile collector. A no-op
// that touches nothing when PROM_TEXTFILE_DIR is unset, the same contract queue-metrics and
// llm-probe keep.
func publishMetrics(unreachable, emptyFeed, boardGone closeReport, applyUnreachable, applyEmptyFeed, applyBoardGone bool) {
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		return
	}
	if err := worker.WriteTextfile(dir, textfileName, render(unreachable, emptyFeed, boardGone, applyUnreachable, applyEmptyFeed, applyBoardGone)); err != nil {
		log.Printf("close-chronic-boards: metrics not published: %v", err)
	}
}
