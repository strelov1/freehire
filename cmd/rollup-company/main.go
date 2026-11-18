// Command rollup-company is the standalone per-company hiring-signal rollup worker.
// It computes insights_company_stats on the analytics reader and publishes it
// atomically.
//
// insights_company_stats holds one row per (company_slug, day) for each of a company's
// activity days, with that day's `added`/`removed` and a running `open` count, derived
// solely from jobs.created_at/closed_at (closed jobs are retained). insights_company_growth
// holds one scalar row per company (open_count now + open_count as of the growth window
// ago) that backs the ranked /api/v1/insights/companies leaderboard. Both are the company-
// grained siblings of the insights_* rollups and the foundation for a company hiring
// signal (who is ramping vs. freezing). insights_company_lifetime holds each company's
// median days from posting to an employer-side closure and insights_company_views its
// postings' page views over the last 30 days and the 30 before, both shown on the
// company page's Hiring tab.
//
// It is a run-once-and-exit worker (cron-scheduled, ~daily given the company grain):
// computation precedes one atomic primary publication transaction, so a reader never sees a table
// mid-rebuild and orphaned rows (e.g. a reopened job) vanish in the same step.
// insights_company_stats, the largest of the five, is published as a difference against
// the stored rows (publishCompanyStats) rather than deleted and copied whole. Kept
// separate from rollup-stats so its heavier, company-grained recompute schedules
// independently and its failure never blocks the public /insights rollups. Re-running
// is safe; it exits non-zero if the rebuild transaction fails, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/platform/worker"
)

// growthWindowDays is how far back insights_company_growth's prior-window open-count
// looks, so growth = open_count - open_count_prev. Matches cmd/rollup-stats.
const growthWindowDays = 30

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

	analytics, err := worker.AnalyticsPool(ctx, pool)
	if err != nil {
		log.Printf("analytics database: %v", err)
		return 1
	}
	defer analytics.Close()
	if err := rebuildCompanyRollups(ctx, analytics, pool); err != nil {
		log.Printf("company rollups: %v", err)
		return 1
	}
	return 0
}
