// Command hydrate-adzuna-description drains the Adzuna description capture queue: for each
// queued posting it fetches the full text from Adzuna's own hosted job page (the API gives
// only a ~500-character snippet) and rewrites the stored description, then exits. Run it on
// a schedule (e.g. cron); it drains what is queued and stops.
//
// cmd/ingest enqueues a job here right after a successful write, but only when its stored
// URL is Adzuna's own hosted page rather than the ad-network tracking redirect (see
// adzunadesc.Eligible) — the tracking shape answers its own "Access Denied" page to a
// request like this one's.
//
// It exits non-zero when the run had any failures or dead letters, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/adzunadesc"
	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/sources"
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

	cfg := config.LoadAdzunaDescription()

	// The same HTTP client the crawl uses: Adzuna's hosted job page is the one endpoint
	// this fleet already talks to for this posting, so its user agent, timeouts and size
	// caps are exactly right here.
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
