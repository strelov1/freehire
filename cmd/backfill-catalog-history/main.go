// Command backfill-catalog-history fills catalog_size_daily for the days before cmd/rollup-stats
// began measuring the catalogue's size. It replays today's jobs as daily flows (created minus
// closed, listed postings only) and adds back the postings the removed cmd/prune hard-deleted,
// dated by their ids. Every row it writes is marked reconstructed: it applies today's
// duplicate verdicts to the past.
//
// The prune also took closed postings and duplicates, and its archive does not say which, so
// -pruned-share (0..1) says what fraction of them to count as listed. It has no default: pick
// it by comparing the dry run with sizes the site actually published at the time.
//
//	backfill-catalog-history -pruned-share 0.5            # dry run: prints the series
//	backfill-catalog-history -pruned-share 0.5 --apply    # writes days no measurement holds
//
// Run-once operator tool; reads on the analytics replica (full scans), needs DATABASE_URL.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	share := flag.Float64("pruned-share", -1, "fraction of pruned postings to count as listed, 0..1 (required)")
	apply := flag.Bool("apply", false, "write the reconstructed days (default: print them)")
	flag.Parse()
	if math.IsNaN(*share) || *share < 0 || *share > 1 {
		log.Printf("backfill-catalog-history: -pruned-share must be set, between 0 and 1")
		return 2
	}

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

	// Through yesterday: today belongs to cmd/rollup-stats, which measures it.
	through := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	points, err := reconstruct(ctx, db.New(analytics), *share, through)
	if err != nil {
		log.Printf("backfill-catalog-history: %v", err)
		return 1
	}
	for _, p := range points {
		if p.Day.Weekday() == time.Monday || p.Day.Equal(through) {
			_, _ = fmt.Fprintf(os.Stdout, "%s\t%d\n", p.Day.Format(time.DateOnly), p.Open)
		}
	}
	if !*apply {
		log.Printf("backfill-catalog-history: dry run, %d days; pass --apply to write them", len(points))
		return 0
	}

	params := db.RecordReconstructedCatalogSizeParams{}
	for _, p := range points {
		params.Days = append(params.Days, pgtype.Date{Time: p.Day, Valid: true})
		params.OpenJobs = append(params.OpenJobs, p.Open)
	}
	if err := db.New(pool).RecordReconstructedCatalogSize(ctx, params); err != nil {
		log.Printf("backfill-catalog-history: write: %v", err)
		return 1
	}
	log.Printf("backfill-catalog-history: wrote up to %d reconstructed days (measured days kept)", len(points))
	return 0
}

func reconstruct(ctx context.Context, q *db.Queries, share float64, through time.Time) ([]point, error) {
	flowRows, err := q.ListedJobDailyFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("daily flow: %w", err)
	}
	firstRows, err := q.FirstJobIDPerDay(ctx)
	if err != nil {
		return nil, fmt.Errorf("first ids: %w", err)
	}
	prunedRows, err := q.PrunedJobIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("pruned ids: %w", err)
	}

	flows := make([]flow, len(flowRows))
	for i, r := range flowRows {
		flows[i] = flow{Day: r.Day.Time, Added: r.Added, Removed: r.Removed}
	}
	firsts := make([]firstID, len(firstRows))
	for i, r := range firstRows {
		firsts[i] = firstID{Day: r.Day.Time, ID: r.FirstID}
	}
	// dayOfID searches by id; ids follow creation order, so this keeps the day order too.
	sort.Slice(firsts, func(i, j int) bool { return firsts[i].ID < firsts[j].ID })
	prunes := make([]pruned, len(prunedRows))
	for i, r := range prunedRows {
		prunes[i] = pruned{ID: r.ID, Day: r.PrunedDay.Time}
	}
	return series(flows, firsts, prunes, share, through), nil
}
