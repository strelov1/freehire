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
const chronicBoardCloseWindowDaysDefault = 60

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually close; without it the run only reports")
	flag.Bool("dry-run", false, "no-op: reporting is the default, --apply is what closes")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	days, err := worker.EnvInt64("CHRONIC_BOARD_CLOSE_WINDOW_DAYS", chronicBoardCloseWindowDaysDefault)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}

	report, err := closeChronicBoards(ctx, db.New(pool), days, maxChronicBoardsPerRun, *apply)
	if err != nil {
		log.Printf("close-chronic-boards: %v", err)
		return 1
	}

	verb := "would close"
	if *apply {
		verb = "closed"
	}
	log.Printf("close-chronic-boards: %d chronic board(s), %s %d job(s) total",
		report.boardsProcessed, verb, report.jobsAffected)
	return 0
}
