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

	runner := searchdrain.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client, q: q},
	}

	opt := searchdrain.RunOptions{
		BatchSize:    dcfg.BatchSize,
		LeaseSeconds: dcfg.LeaseSeconds,
		MaxAttempts:  dcfg.MaxAttempts,
		CallTimeout:  dcfg.CallTimeout,
		MaxPerRun:    dcfg.MaxPerRun,
	}

	// Removals run BEFORE the pushes, and the order is the whole point. Both waves touch the
	// same index and are paused together by freehire-reindexw, so they belong to one worker
	// rather than two units that would each need their own copy of that coupling — but one
	// worker means one of them goes second, and going second here is not a delay, it is
	// starvation.
	//
	// This used to be the other way round, so "a fault in the newer path cannot delay the
	// established one". The reasoning inverted in practice: the indexing pass drains until the
	// queue is EMPTY, and on this fleet the queue need never be empty. Measured on prod
	// 2026-09-06, after a 5h reindex had paused this unit: arrivals 2504/min against a drain of
	// 2614/min, so the pass hovered at a 9-11k depth and did not return for over two hours.
	// Type=oneshot means no second instance starts, so removals had not run once in that window
	// and 134k closed postings stayed visible in search. Indexing was healthy throughout —
	// failed=0, dead=0, 200k+ pushed. The established path did not fault; it simply never
	// finished, which the old ordering had no answer for.
	//
	// Removals cannot starve pushes the same way: a wave is ONE index task for the whole batch,
	// its queue is fed only by closures rather than by every content change, and its own
	// CallTimeout plus dead-lettering bound a wave that goes wrong. What bounds each pass
	// against the other is MaxPerRun (see config.SearchDrain).
	deletions := searchdrain.DeletionRunner{
		Store:   newDeletionStore(pool),
		Deleter: facetDeleter{client: client},
	}
	del, err := deletions.Run(ctx, opt)
	if err != nil {
		log.Printf("search-drain: deletions: %v", err)
		return 1
	}

	stats, err := runner.Run(ctx, opt)
	if err != nil {
		log.Printf("search-drain: %v", err)
		return 1
	}

	log.Printf("search-drain done: indexed=%d deleted=%d failed=%d dead_lettered=%d",
		stats.Indexed, del.Deleted, stats.Failed+del.Failed, stats.DeadLettered+del.DeadLettered)
	return worker.ExitCode(stats.Failed+del.Failed, stats.DeadLettered+del.DeadLettered)
}
