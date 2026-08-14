// Command embed is the standalone incremental semantic-embedding worker. It enqueues
// open jobs whose embedding is missing/stale (and closed jobs whose embed state must
// be cleared), then drains the semantic_outbox queue: each open job is embedded via
// TEI and its chunk rows persisted to Postgres; each closed job has
// its embed state cleared. Run it on a schedule (e.g. cron); it drains what is queued
// and exits. It is the incremental sibling of cmd/enrich. It exits non-zero when the
// run had any failures or dead-letters, so cron can alert.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Unlike cmd/reindex, this worker no longer requires MEILI_MASTER_KEY: it never
	// touches Meilisearch (the jobs_semantic index it used to upsert into is gone —
	// see openspec/changes/drop-hybrid-search-pgvector-similar). search.NewClient still
	// takes cfg.MeiliURL/cfg.MeiliKey positionally, but nothing here exercises the I/O
	// they would configure.
	ecfg := config.LoadEmbed()
	ec := config.LoadEmbedClient()
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey,
		search.WithEmbedURL(ec.URL), search.WithEmbedAPIKey(ec.APIKey), search.WithEmbedConcurrency(ec.Concurrency))

	runner := embed.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client},
	}

	stats, err := runner.Run(ctx, embed.RunOptions{
		TargetModel:  search.CurrentEmbedderModel(),
		BatchSize:    ecfg.BatchSize,
		LeaseSeconds: ecfg.LeaseSeconds,
		MaxAttempts:  ecfg.MaxAttempts,
		CallTimeout:  ecfg.CallTimeout,
	})
	if err != nil {
		log.Printf("embed: %v", err)
		return 1
	}

	log.Printf("embed done: indexed=%d removed=%d failed=%d dead_lettered=%d",
		stats.Indexed, stats.Removed, stats.Failed, stats.DeadLettered)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
