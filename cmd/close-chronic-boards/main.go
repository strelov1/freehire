// Command close-chronic-boards is the safety net for a board (or a boardless provider) that
// has proven unreachable for a long time — not merely cooling down from a recent run of
// failures (openspec change close-chronically-unreachable-boards, issue #2017).
//
// The ordinary per-run unseen sweep (cmd/ingest) deliberately never closes a board's jobs
// unless THAT RUN proved it covered the board — see job-lifecycle spec's shouldSweep /
// boardQualifies / sweepableCompanies. That guard is correct against a transient failure but
// has no upper bound: a board that is permanently gone keeps its stale jobs open forever,
// since it can never re-qualify. This worker is the deliberate, separate escape hatch: once
// board_health.ListChronicBoards proves a board has had no successful crawl for the closure
// window (default 60 days, CHRONIC_BOARD_CLOSE_WINDOW_DAYS overrides), its open jobs are
// closed with their own 'board_unreachable' reason — never conflated with the ordinary sweep's
// 'unseen'.
//
// Dry-run by default, reporting what it would close; --apply writes. Per design.md's Migration
// Plan, ship this dry-run-only on its timer until a human has read at least one full closure
// window's report — a board a curator would rather retire or fix stays a curation decision,
// not a fait accompli.
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// chronicBoardCloseWindowDaysDefault is deliberately longer than the per-run report's chronic
// window (cmd/ingest's chronicBoardWindowDaysDefault, 30 days): a curator who reads that report
// has the gap between the two windows to retire or fix a board before this worker would have
// closed it anyway (design.md Decision 1).
const chronicBoardCloseWindowDaysDefault int32 = 60

// emptyFeedCloseWindowDaysDefault is deliberately SHORTER than the unreachable window above,
// because the two windows are waiting out different uncertainties. An unreachable board is
// waiting out OUR side — a blocked crawler, a rotated URL, an adapter that needs a fix — and
// sixty days is the room a curator is given to act before the safety net does. An empty feed has
// already told us its answer, successfully, on every run: there is nothing here. Thirty days is
// how long we insist it keeps saying so before believing it, which is generous against the one
// benign reading (a seasonal or deliberately paused board) and still an order of magnitude
// tighter than leaving the postings open forever, which is what happened before this existed.
const emptyFeedCloseWindowDaysDefault int32 = 30

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually close chronically UNREACHABLE boards; without it that pass only reports")
	// A second, separate switch rather than letting --apply arm both passes. The unit file's
	// own comment tells a future operator to add --apply once they have read a full closure
	// window's worth of the UNREACHABLE pass's reports — and if that one flag also armed this
	// one, following that instruction would silently start closing jobs on evidence nobody had
	// reviewed, which is precisely the posture the dry-run default exists to prevent. Each
	// safety net is armed by a person who has read that net's own reports.
	applyEmptyFeed := flag.Bool("apply-empty-feed", false, "actually close reachable-but-EMPTY boards; without it that pass only reports")
	flag.Bool("dry-run", false, "no-op: reporting is the default, --apply / --apply-empty-feed are what close")
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

	q := db.New(pool)

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

	// The empty-feed pass runs after the unreachable one and never instead of it: the two select
	// disjoint sets by construction (a board qualifying here must have succeeded inside its own
	// window, which is exactly what a chronic board has not), so neither can take work from the
	// other, and a board that somehow moved between the two states between the passes is simply
	// examined twice against predicates the close statements re-validate anyway.
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
	return 0
}
