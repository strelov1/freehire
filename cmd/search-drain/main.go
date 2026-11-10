// Command search-drain is the standalone incremental facet-search worker. It drains
// the search_outbox queue: each wave of jobs cmd/ingest queued (new or content-
// changed, since its last drain) is pushed into the live Meilisearch `jobs` index as
// ONE batch, then the wave's outbox entries are deleted. Run it on a schedule (e.g.
// cron); it drains what is queued and exits. It is the incremental sibling of
// cmd/embed, and the only routine path to the index: the full `reindex` swap-rebuild is a
// by-hand step after a backfill (internal/search/searchdrain/AGENTS.md). It exits non-zero
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
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

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

	facetd, err := newFacetd(db.New(pool))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	runner := searchdrain.Runner{
		Store:   newDBStore(pool),
		Indexer: searchIndexer{client: client, q: q, mirror: facetd.mirror()},
	}

	opt := searchdrain.RunOptions{
		BatchSize:    dcfg.BatchSize,
		LeaseSeconds: dcfg.LeaseSeconds,
		MaxAttempts:  dcfg.MaxAttempts,
		CallTimeout:  dcfg.CallTimeout,
		MaxPerRun:    dcfg.MaxPerRun,
		Stop:         func() bool { return worker.Paused(ctx, cfg.RedisURL, worker.SearchDrainJob) },
	}

	// Alternate one wave of each queue so an indexing backlog cannot hold up
	// removals. A short wave ends that queue's pass until the next scheduled run.
	deletions := searchdrain.DeletionRunner{
		Store:   newDeletionStore(pool),
		Deleter: facetDeleter{client: client, q: db.New(pool), mirror: facetd.mirror()},
	}
	stats, del, err := searchdrain.Drain(ctx, runner, deletions, opt)
	if err != nil {
		log.Printf("search-drain: %v", err)
		return 1
	}

	facetd.retry(ctx, newDBFacetdRetryStore(pool), opt)

	log.Printf("search-drain done: indexed=%d deleted=%d failed=%d dead_lettered=%d",
		stats.Indexed, del.Deleted, stats.Failed+del.Failed, stats.DeadLettered+del.DeadLettered)
	return worker.ExitCode(stats.Failed+del.Failed, stats.DeadLettered+del.DeadLettered)
}

// facetdSetup is the optional facetd mirror (FACETD_URL); the zero value is "off".
type facetdSetup struct {
	client *search.FacetdClient
	m      *facetdMirror
	q      *db.Queries
}

// newFacetd reads FACETD_URL and FACETD_WRITE_TIMEOUT (default 10s). Unset URL: no facetd
// calls and no queue writes.
func newFacetd(q *db.Queries) (facetdSetup, error) {
	url := os.Getenv("FACETD_URL")
	if url == "" {
		return facetdSetup{}, nil
	}
	timeout := 10 * time.Second
	if v := os.Getenv("FACETD_WRITE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return facetdSetup{}, fmt.Errorf("FACETD_WRITE_TIMEOUT %q: want a positive duration like 10s", v)
		}
		timeout = d
	}
	c := search.NewFacetdClient(url, &http.Client{})
	return facetdSetup{
		client: c,
		q:      q,
		m:      &facetdMirror{client: c, queue: dbFacetdQueue{q}, timeout: timeout},
	}, nil
}

// mirror is nil when facetd is off, which the mirror's methods accept.
func (f facetdSetup) mirror() *facetdMirror { return f.m }

// retry runs the retry pass after the Meilisearch passes and publishes the metrics.
func (f facetdSetup) retry(ctx context.Context, store facetdRetryStore, opt searchdrain.RunOptions) {
	if f.m == nil {
		return
	}
	facetdRetry{store: store, facetd: f.client, build: docBuilder{f.q}.build, mirror: f.m}.Run(ctx, opt)
	publishFacetdMetrics(ctx, store, f.m.failures.Load())
}
