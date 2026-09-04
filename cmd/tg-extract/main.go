// Command tg-extract is the standalone Telegram extraction worker. It drains the
// telegram_posts queue: for each claimed post it asks the LLM to classify the post
// and extract its vacancies, validates the payload, and writes the jobs through
// the canonical upsert — enqueuing them for enrichment in the same transaction as
// marking the post extracted. Run it on a schedule (e.g. cron); it processes a
// bounded batch and exits. It exits non-zero when the run finished with any
// failures, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	// LLM and channel config are loaded first so a misconfigured worker fails before
	// it opens the pool. Its OWN LLM config: this used to be config.LoadEnrich(), read
	// purely to reach the six LLM values — which meant a Telegram extractor inherited the
	// enrichment worker's ENRICH_* validation and would have broken if that changed.
	lcfg := config.LoadLLM()
	if err := lcfg.Require(); err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// One construction path: llm.NewClient builds the client and, when LANGFUSE_* are
	// set, wires tracing (source "telegram"). flush drains buffered traces at run end
	// (no-op when tracing is off). LoadEnrich already required the LLM settings.
	client, flush, err := llm.NewClient(lcfg.Settings(lcfg.Model), "telegram")
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

	// telegram_channels supplies each channel's kind, steering the extraction prompt.
	chanCfg, err := telegram.LoadChannels(ctx, db.New(pool))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	kinds := chanCfg.Kinds()

	extractor := telegram.NewLangChainExtractor(client)

	runner := telegram.ExtractRunner{
		Extractor: extractor,
		Store:     newExtractStore(pool),
		Kinds:     kinds,
		Links:     linkResolver{reg: linksource.All(sources.NewClient())},
	}

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("extract: %v", err)
		return 1
	}
	// skipped is the mis-extraction count: vacancies the model produced that the domain
	// refused for having no title or identity. It used to be invisible — the adapter dropped
	// them and jobs counted them anyway — so a run whose extraction was degrading looked
	// exactly like a healthy one.
	log.Printf("tg-extract done: processed=%d jobs=%d skipped=%d failed=%d",
		stats.Processed, stats.Jobs, stats.Skipped, stats.Failed)
	return worker.ExitCode(stats.Failed, 0)
}
