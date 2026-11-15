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

	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/application/maillink"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	// LLM config first, so a misconfigured worker fails before it opens the pool. Its own
	// config, not LoadEnrich, so ENRICH_* validation does not leak in.
	lcfg := config.LoadLLM()
	if err := lcfg.Require(); err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	client, flush, err := llm.NewClient(lcfg.Settings(lcfg.Model), "classify-mail")
	if err != nil {
		log.Printf("llm: %v", err)
		return 1
	}
	defer flush()

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	runner := maillink.New(newDBStore(pool), mailclassify.NewClassifier(client, cfg.TypesafeAPIKey), client.ModelID()).
		WithLearner(newDomainLearner(pool))
	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("classify-mail: %v", err)
		return 1
	}
	log.Printf("classify-mail: done failed=%d dead-lettered=%d", stats.Failed, stats.DeadLettered)
	// A queue that dead-letters every entry must show up as a non-zero exit; nothing else
	// reads email_classification_outbox.failed_at.
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
