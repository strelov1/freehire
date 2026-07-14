// Command classify-mail is the standalone email-classification worker. It enqueues
// every unclassified inbox email, then drains the outbox: for each email it resolves
// the application it belongs to (deterministic mailmatch cascade, LLM for the tail)
// and classifies its status, persisting the confidence-tiered link and a
// monotonic-forward stage advance. Run it on a schedule; it processes the backlog
// and exits.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/mailclassify"
	"github.com/strelov1/freehire/internal/maillink"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	// LLM config first, so a misconfigured worker fails before it opens the pool.
	ecfg, err := config.LoadEnrich()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	client, flush, err := llm.NewClient(llm.Settings{
		BaseURL:           ecfg.LLMBaseURL,
		APIKey:            ecfg.LLMAPIKey,
		Model:             ecfg.LLMModel,
		LangfuseBaseURL:   ecfg.LangfuseBaseURL,
		LangfusePublicKey: ecfg.LangfusePublicKey,
		LangfuseSecretKey: ecfg.LangfuseSecretKey,
	}, "classify-mail")
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

	runner := maillink.New(newDBStore(pool), mailclassify.NewClassifier(client), client.ModelID())
	if err := runner.Run(ctx); err != nil {
		log.Printf("classify-mail: %v", err)
		return 1
	}
	log.Printf("classify-mail: done")
	return 0
}
