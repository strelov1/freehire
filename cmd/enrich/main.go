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

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	os.Exit(run())
}

func run() int {
	// LLM config is loaded first so a misconfigured worker fails before it opens
	// the pool.
	ecfg, err := config.LoadEnrich()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// Optional Langfuse tracing: nil (no-op) unless LANGFUSE_* are set. flush drains
	// buffered generations at the end of the run.
	tracer, flush := worker.Tracing(ecfg)
	defer flush()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	provider, err := enrich.NewLangChainProvider(ecfg.LLMBaseURL, ecfg.LLMAPIKey, ecfg.LLMModel, llm.WithTracer(tracer, "enrich"))
	if err != nil {
		log.Printf("provider: %v", err)
		return 1
	}

	runner := enrich.Runner{Provider: provider, Store: newDBStore(pool)}

	stats, err := runner.Run(ctx, enrich.RunOptions{
		TargetVersion: enrich.Version,
		Concurrency:   ecfg.Concurrency,
		LeaseSeconds:  ecfg.LeaseSeconds,
		MaxAttempts:   ecfg.MaxAttempts,
	})
	if err != nil {
		log.Printf("enrich: %v", err)
		return 1
	}

	log.Printf("enrichment done: enriched=%d failed=%d dead_lettered=%d",
		stats.Enriched, stats.Failed, stats.DeadLettered)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
