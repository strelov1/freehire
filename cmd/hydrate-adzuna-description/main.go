// Command hydrate-adzuna-description drains the Adzuna description capture queue: for each
// queued posting it fetches the full text from Adzuna's hosted job page (the API gives only
// a snippet) and rewrites the stored description, then exits. Run it on a schedule.
//
// cmd/ingest enqueues a job only when its URL is Adzuna's hosted page, not the ad-network
// tracking redirect (see adzunadesc.Eligible), which answers "Access Denied".
//
// It exits non-zero when the queue is unusable, or when a run captured nothing but had failures or
// dead letters (RunStats.Degraded), so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/adzunadesc"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/worker"
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

	cfg := config.LoadAdzunaDescription()

	// The same HTTP client the crawl uses, so its user agent, timeouts and size caps apply.
	client := sources.NewClient()
	fetch := func(ctx context.Context, url string) (string, error) {
		return adzunadesc.FetchDescription(ctx, client, url)
	}

	stats, err := adzunadesc.Run(ctx, newDBStore(pool), fetch, adzunadesc.RunOptions{
		BatchSize:    cfg.BatchSize,
		LeaseSeconds: cfg.LeaseSeconds,
		MaxAttempts:  cfg.MaxAttempts,
		Concurrency:  cfg.Concurrency,
		MaxPerRun:    cfg.MaxPerRun,
		CallTimeout:  cfg.CallTimeout,
	})
	if err != nil {
		log.Printf("hydrate-adzuna-description: %v", err)
		return 1
	}

	log.Printf("hydrate-adzuna-description done: captured=%d failed=%d skipped=%d dead_lettered=%d",
		stats.Captured, stats.Failed, stats.Skipped, stats.DeadLettered)
	if stats.Degraded() {
		return 1
	}
	return 0
}
