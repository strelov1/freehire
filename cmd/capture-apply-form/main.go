// Command capture-apply-form drains the apply-form capture queue: for each queued posting
// it fetches the application form from the ATS that published it and stores it, then
// exits. Run it on a schedule (e.g. cron); it drains what is queued and stops.
//
// It is the second half of a deliberate split. Recruitee's form arrives inside the board
// listing ingest already downloads and is written during the crawl; Greenhouse and Ashby
// expose theirs only per posting, and fetching that mid-crawl would turn one board request
// into hundreds while the adapter still could not tell which postings are new — that
// answer exists only after the upsert resolves. So the crawl enqueues and this drains.
//
// It exits non-zero when the run had any failures or dead letters, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/applyform"
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

	cfg := config.LoadApplyForm()

	// The crawl's HTTP client: the endpoints are the platforms' own public job-board APIs, so
	// its user agent, timeouts and size caps are exactly right here. Every request egresses
	// through the proxy — this worker is what drove Workable into banning the host's own
	// address for 23 hours on 2026-10-25 (sources.NewFormClientFromEnv).
	client, proxied, err := sources.NewFormClientFromEnv()
	if err != nil {
		log.Printf("capture-apply-form: %v", err)
		return 1
	}
	if !proxied {
		log.Printf("capture-apply-form: WARNING no SOURCES_ROTATING_PROXY_URL or SOURCES_PROXY_URL — forms are fetched from this host's own address")
	}
	stats, err := applyform.Run(ctx, newDBStore(pool), applyform.Fetchers(client), applyform.RunOptions{
		BatchSize:    cfg.BatchSize,
		LeaseSeconds: cfg.LeaseSeconds,
		MaxAttempts:  cfg.MaxAttempts,
		Concurrency:  cfg.Concurrency,
		MaxPerRun:    cfg.MaxPerRun,
		CallTimeout:  cfg.CallTimeout,
	})
	if err != nil {
		log.Printf("capture-apply-form: %v", err)
		return 1
	}

	log.Printf("capture-apply-form done: captured=%d gone=%d failed=%d dead_lettered=%d deferred=%d",
		stats.Captured, stats.Gone, stats.Failed, stats.DeadLettered, stats.Deferred)
	// Deliberately not worker.ExitCode: see RunStats.Degraded for why a few retryable
	// failures are the healthy shape of this particular worker rather than a bad run.
	if stats.Degraded() {
		return 1
	}
	return 0
}
