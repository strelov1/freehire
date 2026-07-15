// Command embed is the standalone incremental semantic-embedding worker. It enqueues
// open jobs whose vector is missing/stale (and closed jobs whose vector must be
// removed), then drains the semantic_outbox queue: each open job is embedded and its
// vector upserted into jobs_semantic IN PLACE (no swap), each closed job's document is
// removed. Run it on a schedule (e.g. cron); it drains what is queued and exits. It is
// the incremental sibling of cmd/enrich; the full `reindex --semantic` swap-rebuild
// stays the reconciler. It exits non-zero when the run had any failures or
// dead-letters, so cron can alert.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/db"
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

	// Bootstrap owns config + pool, so this required-config check lands just after the
	// pool opens (mirrors cmd/reindex). Without a Meili key the worker has nothing to
	// index into.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	// EMBED_PG_ONLY drains the queue writing vectors to Postgres ONLY (no Meili), for a
	// fast bulk backfill that Meili's serial task queue can't gate; rebuild the index
	// afterwards with `reindex --semantic --from-pg`.
	pgOnly := os.Getenv("EMBED_PG_ONLY") != ""

	ecfg := config.LoadEmbed()
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	// Ensure the semantic index exists WITH its userProvided embedder before pushing
	// vectors. Unlike the incremental facet path (a plain index), a semantic upsert into
	// an index that lacks the embedder settings fails every task. `reindex --semantic` is
	// the usual creator, but this makes the worker self-sufficient on a fresh index (e.g.
	// the embed cron firing before the first semantic reindex). Idempotent. pg-only never
	// writes Meili, so it skips this (the only startup Meili call).
	if pgOnly {
		log.Print("embed: PG-ONLY mode — writing vectors to Postgres, NOT Meili")
	} else if err := client.EnsureSemanticIndex(ctx); err != nil {
		log.Printf("search: ensure semantic index: %v", err)
		return 1
	}

	runner := embed.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client, q: db.New(pool), pgOnly: pgOnly},
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
