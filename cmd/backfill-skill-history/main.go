// Command backfill-skill-history retroactively fills insights_skill_history for
// past ISO weeks, so a newly-shipped skill's demand trend doesn't have to grow
// one live snapshot at a time from an empty table.
//
// Each past week's open_count is reconstructed directly from
// jobs.created_at/closed_at — the same open_at(D) formula
// RebuildInsightsSkillStatsGlobal already trusts for its 30-day-back
// comparison — evaluated at that week's Monday, midnight UTC, instead of "now".
//
// ON CONFLICT DO NOTHING (in the query, not here) makes this idempotent and
// safe to run alongside the live cmd/rollup-stats writer: a week the live
// writer already recorded is never overwritten by a backfilled one, so a
// backfill run can be repeated or interrupted without repair.
//
// -weeks bounds how many past ISO weeks to fill (default 26, matching
// cmd/rollup-stats' retention); the current week is never touched here — the
// live writer owns it.
// -dry-run runs every insert inside a transaction and rolls it back, so it
// reports exactly what would be written and leaves nothing behind.
//
// Run: DATABASE_URL=... go run ./cmd/backfill-skill-history [-weeks 26] [-dry-run]
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/isoweek"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	weeks := flag.Int("weeks", 26, "how many past ISO weeks to backfill")
	dryRun := flag.Bool("dry-run", false, "report what would be inserted without writing")
	flag.Parse()

	worker.Main(func() int { return run(*weeks, *dryRun) })
}

func run(weeks int, dryRun bool) int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)

	// A dry run executes the real inserts inside a transaction and rolls it back —
	// the count IS the real pass, measured then discarded (mirrors
	// cmd/backfill-application-events' dry-run shape).
	var tx pgx.Tx
	if dryRun {
		if tx, err = pool.Begin(ctx); err != nil {
			log.Printf("dry run: begin: %v", err)
			return 1
		}
		defer func() { _ = tx.Rollback(ctx) }()
		queries = queries.WithTx(tx)
	}

	// Start from LAST week (i=1), never the current one — that week belongs to the
	// live rollup-stats writer, and ON CONFLICT DO NOTHING would just no-op on it
	// anyway, but skipping it outright keeps this worker's job description honest.
	currentWeek := isoweek.Start(time.Now().UTC())
	var total int64
	for i := 1; i <= weeks; i++ {
		asOf := currentWeek.AddDate(0, 0, -7*i)
		n, err := queries.BackfillInsightsSkillHistoryWeek(ctx, db.BackfillInsightsSkillHistoryWeekParams{
			WeekStart: pgtype.Date{Time: asOf, Valid: true},
			AsOf:      pgtype.Timestamptz{Time: asOf, Valid: true},
		})
		if err != nil {
			log.Printf("week %s: %v", asOf.Format("2006-01-02"), err)
			return 1
		}
		total += n
		log.Printf("week %s: %d skill row(s)", asOf.Format("2006-01-02"), n)
	}

	if dryRun {
		log.Printf("backfill dry run: would record %d skill-week row(s) across %d week(s)", total, weeks)
		return 0
	}
	log.Printf("backfill complete: recorded %d skill-week row(s) across %d week(s)", total, weeks)
	return 0
}
