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

	"github.com/strelov1/freehire/internal/applyform"
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

	cfg := config.LoadApplyForm()

	// The same HTTP client the crawl uses: the endpoints are the platforms' own public
	// job-board APIs, so its user agent, timeouts and size caps are exactly right here.
	stats, err := applyform.Run(ctx, newDBStore(pool), applyform.Fetchers(sources.NewClient()), applyform.RunOptions{
		BatchSize:    cfg.BatchSize,
		LeaseSeconds: cfg.LeaseSeconds,
		MaxAttempts:  cfg.MaxAttempts,
		Concurrency:  cfg.Concurrency,
		CallTimeout:  cfg.CallTimeout,
	})
	if err != nil {
		log.Printf("capture-apply-form: %v", err)
		return 1
	}

	log.Printf("capture-apply-form done: captured=%d failed=%d dead_lettered=%d",
		stats.Captured, stats.Failed, stats.DeadLettered)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
