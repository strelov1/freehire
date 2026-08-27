// Command search-drain is the standalone incremental facet-search worker. It drains
// the search_outbox queue: each wave of jobs cmd/ingest queued (new or content-
// changed, since its last drain) is pushed into the live Meilisearch `jobs` index as
// ONE batch, then the wave's outbox entries are deleted. Run it on a schedule (e.g.
// cron); it drains what is queued and exits. It is the incremental sibling of
// cmd/embed; the full `reindex` swap-rebuild stays the reconciler. It exits non-zero
// when the run had any failures or dead-letters, so cron can alert.
//
// This replaces cmd/ingest's previous inline push (search.Client.SubmitJobs called
// once per ingest run): with ~169 independent per-board ingest processes, that meant
// one Meili task per board per crawl, each forcing a full index re-merge (observed at
// 50-90s regardless of batch size on the production catalogue) — the dominant cause
// of a prod disk-IO saturation that produced nginx 504s. Routing every write through
// one outbox drained on its own schedule collapses however many boards changed in a
// window into one Meili task.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
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
	// pool opens (mirrors cmd/embed). Without a Meili key the worker has nothing to
	// index into.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	dcfg := config.LoadSearchDrain()
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	q := db.New(pool)

	// One snapshot of the match sort's rarity weights per run. Not fatal on failure:
	// the drain's job is to keep the index current, and dropping this wave's vectors is
	// a far smaller loss than dropping the wave. The next full rebuild rewrites them.
	skillWeights, err := search.LoadSkillWeights(ctx, q)
	if err != nil {
		log.Printf("search-drain: skill weights unavailable, this run's documents carry no skill vector: %v", err)
	}

	runner := searchdrain.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client, q: q, skillWeights: skillWeights},
	}

	opt := searchdrain.RunOptions{
		BatchSize:    dcfg.BatchSize,
		LeaseSeconds: dcfg.LeaseSeconds,
		MaxAttempts:  dcfg.MaxAttempts,
		CallTimeout:  dcfg.CallTimeout,
	}

	stats, err := runner.Run(ctx, opt)
	if err != nil {
		log.Printf("search-drain: %v", err)
		return 1
	}

	// Removals run after the pushes, in the same pass. Both waves touch the same index and
	// are paused together by freehire-reindexw, so they belong to one worker rather than two
	// units that would each need their own copy of that coupling. Indexing goes first so a
	// fault in the newer path cannot delay the established one.
	deletions := searchdrain.DeletionRunner{
		Store:   newDeletionStore(pool),
		Deleter: facetDeleter{client: client},
	}
	del, err := deletions.Run(ctx, opt)
	if err != nil {
		log.Printf("search-drain: deletions: %v", err)
		return 1
	}

	log.Printf("search-drain done: indexed=%d deleted=%d failed=%d dead_lettered=%d",
		stats.Indexed, del.Deleted, stats.Failed+del.Failed, stats.DeadLettered+del.DeadLettered)
	return worker.ExitCode(stats.Failed+del.Failed, stats.DeadLettered+del.DeadLettered)
}
