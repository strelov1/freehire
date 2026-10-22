// Command enrich is the standalone enrichment worker. It enqueues jobs that need
// enriching, then drains the outbox queue: for each claimed job it asks the LLM for
// a structured Enrichment, validates it, and writes it back. Run it on a schedule
// (e.g. cron); it processes a bounded batch and exits. It exits non-zero when the
// run finished with any failures or dead-letters, so cron can alert.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	// LLM config is loaded first so a misconfigured worker fails before it opens
	// the pool.
	ecfg, err := config.LoadEnrich()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// One construction path: llm.NewClient builds the client and, when LANGFUSE_* are
	// set, wires tracing (source "enrich"). flush drains buffered traces at run end
	// (no-op when tracing is off). LoadEnrich already required the LLM settings.
	client, flush, err := llm.NewClient(ecfg.Settings(ecfg.Model), "enrich")
	if err != nil {
		log.Printf("llm: %v", err)
		return 1
	}
	defer flush()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	provider := enrich.NewLangChainProvider(client)

	// The drain runs for days and the per-run metrics only land on exit, so the
	// throughput counter is republished every minute while it works, and once more
	// at exit so the final value is not lost.
	progress := &enrich.Progress{}
	promDir := os.Getenv(worker.PromTextfileDirEnv)
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		publishProgress(promDir, progress.Completed())
	})
	defer func() {
		stopHeartbeat()
		publishProgress(promDir, progress.Completed())
	}()

	runner := enrich.Runner{Provider: provider, Store: newDBStore(pool), Progress: progress}

	stats, err := runner.Run(ctx, enrich.RunOptions{
		TargetVersion:     enrich.Version,
		Concurrency:       ecfg.Concurrency,
		LeaseSeconds:      ecfg.LeaseSeconds,
		MaxAttempts:       ecfg.MaxAttempts,
		UpstreamGraceDays: ecfg.UpstreamGraceDays,
	})
	if err != nil {
		log.Printf("enrich: %v", err)
		return 1
	}

	log.Printf("enrichment done: enriched=%d failed=%d dead_lettered=%d reaped=%d",
		stats.Enriched, stats.Failed, stats.DeadLettered, stats.Reaped)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
