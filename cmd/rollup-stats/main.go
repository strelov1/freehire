// Command rollup-stats is the standalone daily-activity rollup worker. It fully
// recomputes job_daily_stats from the jobs table — one row per UTC calendar day
// holding that day's `added` (jobs created) and `removed` (jobs whose current
// closed_at falls on it) — and swaps it in atomically.
//
// It is a run-once-and-exit worker (cron-scheduled intra-day, ~every few hours,
// so the current day's bar stays fresh without a live-overlay query): clear the
// rollup and re-insert every active day inside one transaction, so a reader of
// GET /api/v1/stats/jobs-activity never sees the table mid-rebuild and a reopened
// job (closed_at cleared) drops out of its old removed day. Re-running is safe —
// the rollup is a pure function of current jobs state. It exits non-zero if the
// rebuild transaction fails, so cron can alert.
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
	// seeing the previous rollup until commit, and orphaned days vanish in the same step.
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Printf("begin: %v", err)
		return 1
	}
	defer tx.Rollback(ctx)
	q := db.New(pool).WithTx(tx)

	if err := q.DeleteAllJobDailyStats(ctx); err != nil {
		log.Printf("clear rollup: %v", err)
		return 1
	}
	days, err := q.RebuildJobDailyStats(ctx)
	if err != nil {
		log.Printf("rebuild rollup: %v", err)
		return 1
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("commit: %v", err)
		return 1
	}

	log.Printf("rollup-stats: rebuilt job_daily_stats (%d active day rows)", days)
	return 0
}
