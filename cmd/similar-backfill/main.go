// Command similar-backfill is the run-once-and-exit worker that populates
// jobs.similar_job_ids/similar_computed_at from the pgvector nearest-neighbour-over-
// chunks rollup (db.NearestJobsToJob) — the precompute step that lets GET
// /jobs/:slug/similar become a plain indexed Postgres lookup instead of a live vector
// query (openspec/changes/drop-hybrid-search-pgvector-similar/design.md Decisions
// 3/4/5).
//
// It mirrors ../telagon/server/cmd/similar-backfill, the original inspiration for
// this worker: find jobs missing a result, process them (in parallel, -workers wide),
// write, repeat. Unlike cmd/embed there is no outbox table to drain (design.md
// Decision 4) — internal/similarjobs.Runner finds outstanding work with a direct
// query instead, so this command only wires flags to it and calls Run once.
//
// Run it on a schedule (e.g. daily cron, per design.md's Open Questions — similarity
// lists don't need same-day freshness the way facets do); it drains what's
// outstanding (or up to -limit jobs) and exits. It exits non-zero when the run had any
// per-job failures, so cron can alert; a failed job is simply retried on the next
// scheduled run (see internal/similarjobs' package doc — no dead-letter machinery).
package main

import (
	"context"
	"flag"
	"log"
	"math"

	"github.com/strelov1/freehire/internal/similarjobs"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	batch := flag.Int("batch", 200, "jobs to fetch per PendingJobIDs page")
	workers := flag.Int("workers", 4, "jobs processed in parallel per batch (each job is three independent Postgres round trips, not a call to a shared external index)")
	limit := flag.Int("limit", 0, "max jobs to process this run (0 = unlimited, drain until nothing outstanding)")
	similarCount := flag.Int("similar", 20, "similar job ids to compute and store per job (>= the API's maxSimilarLimit so a future cap increase needs no re-backfill)")
	flag.Parse()

	// batch/similarCount become int32 Postgres query params (store.go): a value above
	// math.MaxInt32 would wrap silently. -similar=0 is worse than a no-op — it would
	// have NearestJobsToJob return zero neighbours per job and SetSimilarJobIDs still
	// stamp similar_computed_at, permanently marking every processed job "done" with an
	// empty similar-jobs list until someone notices and manually re-nulls the stamp.
	if *batch < 1 || *batch > math.MaxInt32 {
		log.Printf("similar-backfill: -batch must be between 1 and %d, got %d", math.MaxInt32, *batch)
		return 1
	}
	if *similarCount < 1 || *similarCount > math.MaxInt32 {
		log.Printf("similar-backfill: -similar must be between 1 and %d, got %d", math.MaxInt32, *similarCount)
		return 1
	}
	if *limit < 0 {
		log.Printf("similar-backfill: -limit must be >= 0, got %d", *limit)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	runner := similarjobs.Runner{Store: newDBStore(pool)}
	stats, err := runner.Run(ctx, similarjobs.RunOptions{
		BatchSize:      *batch,
		NeighbourCount: *similarCount,
		Concurrency:    *workers,
		MaxJobs:        *limit,
	})
	if err != nil {
		log.Printf("similar-backfill: %v", err)
		return 1
	}

	log.Printf("similar-backfill done: processed=%d stale=%d failed=%d", stats.Processed, stats.Stale, stats.Failed)
	return worker.ExitCode(stats.Failed, 0)
}
