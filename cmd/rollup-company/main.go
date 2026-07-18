// Command rollup-company is the standalone per-company hiring-signal rollup worker.
// It fully recomputes insights_company_stats from the jobs table and swaps it in
// atomically.
//
// insights_company_stats holds one row per (company_slug, day) for each of a company's
// activity days, with that day's `added`/`removed` and a running `open` count, derived
// solely from jobs.created_at/closed_at (closed jobs are retained). It is the company-
// grained sibling of the insights_* rollups and the foundation for a company hiring
// signal (who is ramping vs. freezing).
//
// It is a run-once-and-exit worker (cron-scheduled, ~daily given the company grain):
// the clear and rebuild run inside one transaction, so a reader never sees a table
// mid-rebuild and orphaned rows (e.g. a reopened job) vanish in the same step. Kept
// separate from rollup-stats so its heavier, company-grained recompute schedules
// independently and its failure never blocks the public /insights rollups. Re-running
// is safe; it exits non-zero if the rebuild transaction fails, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// The clear + rebuild run in one transaction so the swap is atomic: readers keep
	// seeing the previous rollup until commit, and orphaned rows vanish in the same step.
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Printf("begin: %v", err)
		return 1
	}
	defer tx.Rollback(ctx)

	// The rebuild aggregates the whole jobs lifecycle with a window sum; at the
	// OLTP-default work_mem its sort/hash spills to disk and drags. Raise work_mem for
	// this batch transaction only (SET LOCAL — reset on commit) so it stays in memory.
	if _, err := tx.Exec(ctx, "SET LOCAL work_mem = '256MB'"); err != nil {
		log.Printf("set work_mem: %v", err)
		return 1
	}

	q := db.New(pool).WithTx(tx)

	if err := q.DeleteAllInsightsCompanyStats(ctx); err != nil {
		log.Printf("clear rollup: %v", err)
		return 1
	}
	rows, err := q.RebuildInsightsCompanyStats(ctx)
	if err != nil {
		log.Printf("rebuild rollup: %v", err)
		return 1
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("commit: %v", err)
		return 1
	}

	log.Printf("rollup-company: rebuilt insights_company_stats (%d company-day rows)", rows)
	return 0
}
