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
	// LLM config first, so a misconfigured worker fails before it opens the pool. Its OWN
	// config: this used to be config.LoadEnrich(), read purely to reach the six LLM values
	// — which meant a classifier inherited the enrichment worker's ENRICH_* validation and would
	// have broken if that changed.
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

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	runner := maillink.New(newDBStore(pool), mailclassify.NewClassifier(client), client.ModelID()).
		WithLearner(newDomainLearner(pool))
	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("classify-mail: %v", err)
		return 1
	}
	log.Printf("classify-mail: done failed=%d dead-lettered=%d", stats.Failed, stats.DeadLettered)
	// Nothing else reads email_classification_outbox.failed_at, so a queue that dead-letters
	// every entry was previously visible only in journalctl. The exit code is what the
	// scheduler sees.
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
