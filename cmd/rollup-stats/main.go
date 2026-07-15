// Command rollup-stats is the standalone daily-activity rollup worker. It fully
// recomputes job_daily_stats and the Trends & Insights rollups (insights_*) from
// the jobs table and swaps them in atomically.
//
// job_daily_stats holds one row per UTC calendar day with that day's `added` (jobs
// created) and `removed` (jobs whose current closed_at falls on it). The insights_*
// tables hold role demand, skill demand, salary bands, and faceted hiring velocity
// that back GET /api/v1/insights/*. Every rollup is a pure function of current jobs
// state — role/skill growth compares open-now against open-as-of a fixed window
// (growthWindowDays), and salary bands are per (currency, period) with a minimum
// sample floor (minSalarySample).
//
// It is a run-once-and-exit worker (cron-scheduled intra-day, ~every few hours, so
// the current day's data stays fresh without a live-overlay query): every clear and
// rebuild runs inside one transaction, so a reader never sees a table mid-rebuild
// and orphaned rows (e.g. a reopened job) vanish in the same step. Re-running is
// safe. It exits non-zero if the rebuild transaction fails, so cron can alert.
package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

const (
	// growthWindowDays is how far back the role/skill "prior window" open-count looks,
	// so growth = open_now - open_as_of(now - growthWindowDays).
	growthWindowDays = 30
	// minSalarySample is the smallest number of disclosed salaries a (currency, period)
	// band needs to be published — below it the band is suppressed as unreliable and
	// potentially individually identifying.
	minSalarySample = 5
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
	// seeing the previous rollups until commit, and orphaned rows vanish in the same step.
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

	if err := rebuildInsights(ctx, q); err != nil {
		log.Printf("rebuild insights: %v", err)
		return 1
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("commit: %v", err)
		return 1
	}

	log.Printf("rollup-stats: rebuilt job_daily_stats (%d active day rows) and insights_* rollups", days)
	return 0
}

// rebuildInsights clears and recomputes the four insights_* rollups inside the
// caller's transaction. prevTs (the growth-window start) and minSalarySample are
// passed to the SQL so the window and sample floor live here, not in the queries.
func rebuildInsights(ctx context.Context, q *db.Queries) error {
	prevTs := pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -growthWindowDays), Valid: true}

	if err := q.DeleteAllInsightsRoleStats(ctx); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsRoleStatsGlobal(ctx, prevTs); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsRoleStatsByCountry(ctx, prevTs); err != nil {
		return err
	}

	if err := q.DeleteAllInsightsSkillStats(ctx); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsSkillStatsGlobal(ctx, prevTs); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsSkillStatsByCategory(ctx, prevTs); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsSkillStatsByCountry(ctx, prevTs); err != nil {
		return err
	}

	if err := q.DeleteAllInsightsSalaryStats(ctx); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsSalaryStatsGlobal(ctx, minSalarySample); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsSalaryStatsByCountry(ctx, minSalarySample); err != nil {
		return err
	}

	if err := q.DeleteAllInsightsVelocityDaily(ctx); err != nil {
		return err
	}
	if _, err := q.RebuildInsightsVelocityDaily(ctx); err != nil {
		return err
	}
	return nil
}
