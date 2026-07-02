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
	"os"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/telegram"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	os.Exit(run())
}

func run() int {
	// LLM and channel config are loaded first so a misconfigured worker fails before
	// it opens the pool.
	ecfg, err := config.LoadEnrich()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// sources/telegram.yml supplies each channel's kind, steering the extraction prompt.
	chanCfg, err := telegram.LoadChannels()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	kinds := chanCfg.Kinds()

	// One construction path: llm.NewClient builds the client and, when LANGFUSE_* are
	// set, wires tracing (source "telegram"). flush drains buffered traces at run end
	// (no-op when tracing is off). LoadEnrich already required the LLM settings.
	client, flush, err := llm.NewClient(llm.Settings{
		BaseURL:           ecfg.LLMBaseURL,
		APIKey:            ecfg.LLMAPIKey,
		Model:             ecfg.LLMModel,
		LangfuseBaseURL:   ecfg.LangfuseBaseURL,
		LangfusePublicKey: ecfg.LangfusePublicKey,
		LangfuseSecretKey: ecfg.LangfuseSecretKey,
	}, "telegram")
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
	log.Printf("tg-extract done: processed=%d jobs=%d failed=%d",
		stats.Processed, stats.Jobs, stats.Failed)
	return worker.ExitCode(stats.Failed, 0)
}
