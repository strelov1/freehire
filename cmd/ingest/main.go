// Command ingest is the standalone source-ingest worker. It loads one board file
// (sources/<provider>.yml, or a mixed sources/custom.yml — passed as the first argument
// or via SOURCES_FILE), fetches each board through its adapter, normalizes the postings,
// and upserts them — enqueuing new ones for enrichment in the same write. A file's
// entries usually share the file-name provider, but an entry may name its own, so one
// file can cover several single-source providers. After the run each provider that
// ingested at least one job has its stale jobs swept. Run on a schedule (e.g. cron); it
// processes its boards once and exits.
//
// When a search engine is configured (MEILI_MASTER_KEY set), each crawl's new or
// content-changed jobs are pushed to the live facet search index, batched, so they
// are searchable within one crawl cycle instead of waiting for the next full
// reindex. The push is best-effort — a search-engine failure is logged and never
// fails the run — and the batch reindex stays the index's source of truth.
package main

import (
	"context"
	"log"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// staleAfter is the grace window before an unseen job is closed: many crawl cycles
// at the hourly per-provider cadence, so a board failing several runs in a row keeps
// its jobs open.
const staleAfter = 48 * time.Hour

func main() {
	os.Exit(run())
}

func run() int {
	// The board file is usually one provider's list (sources/<provider>.yml, provider =
	// file name), but an entry may name its own provider, so it may be a mixed file
	// (sources/custom.yml). Accept it as the first argument (cron passes the file) or via
	// SOURCES_FILE.
	path := os.Getenv("SOURCES_FILE")
	if len(os.Args) > 1 && os.Args[1] != "" {
		path = os.Args[1]
	}
	if path == "" {
		log.Print("config: no board file given (pass sources/<provider>.yml as an argument or set SOURCES_FILE)")
		return 1
	}
	sourceCfg, err := sources.LoadConfig(path)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	registry := sources.All(sources.NewClient())
	// Fail fast before touching the DB: a misconfigured board should not start a run.
	if err := sourceCfg.Validate(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Incremental search indexing is wired only when the search engine is
	// configured for this worker (MEILI_MASTER_KEY set, mirroring the server's
	// search-enabled gate). Absent it, the store gets a nil indexer and ingest runs
	// exactly as before. The full batch reindex stays the index's source of truth.
	var indexer *batchIndexer
	var storeIndexer jobIndexer
	if cfg.MeiliKey != "" {
		client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
		indexer = newBatchIndexer(client.SubmitJobs, indexChunkSize)
		storeIndexer = indexer
	}

	runner := pipeline.Runner{
		Registry: registry,
		Store:    newDBStore(pool, enrich.Version, storeIndexer),
	}

	runStats, err := runner.Run(ctx, sourceCfg.Sources)
	if err != nil {
		log.Printf("ingest: %v", err)
		return 1
	}

	// Flush whatever new/changed documents the crawl buffered into the live index.
	// Best-effort: failures are already logged per batch and never affect the run.
	if indexer != nil {
		indexer.Flush(ctx)
		st := indexer.Stats()
		log.Printf("ingest index: indexed=%d failed=%d", st.Indexed, st.Failed)
	}

	total := runStats.Total()
	log.Printf("ingest done: file=%s providers=%d ingested=%d failed=%d skipped=%d",
		path, len(runStats), total.Ingested, total.Failed, total.Skipped)

	// A failed board is counted in total.Failed; surface it (and any sweep failure
	// below) through the exit code so cron alerts on a degraded run.
	failed := total.Failed

	// Post-run sweep (job-lifecycle spec): per provider, close that provider's open jobs
	// unseen for the whole grace window. Scoped per provider so one provider's run never
	// closes another's jobs, and guarded per provider (only those that ingested at least
	// one job) so a total crawl outage for one provider cannot mass-close its catalogue —
	// even when several providers share one run (e.g. custom.yml).
	queries := db.New(pool)
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-staleAfter), Valid: true}
	// A self-closing source (e.g. jobtech) manages its own closes from its stream, so the
	// unseen sweep must skip it: it re-reports only changed ads, and the cutoff would wrongly
	// close every still-open ad it did not touch this run.
	selfClosing := make(map[string]bool)
	for _, p := range sources.SelfClosingProviders(registry) {
		selfClosing[p] = true
	}
	for _, provider := range sweepableProviders(runStats) {
		if selfClosing[provider] {
			continue
		}
		closed, err := queries.CloseUnseenJobs(ctx, db.CloseUnseenJobsParams{
			Source: provider,
			Cutoff: cutoff,
		})
		if err != nil {
			// Count and continue: one provider's sweep failure must not skip the rest,
			// but the run still exits non-zero.
			failed++
			log.Printf("close stale jobs (%s): %v", provider, err)
			continue
		}
		log.Printf("closed %d stale %s jobs (unseen for %s)", closed, provider, staleAfter)
	}
	return worker.ExitCode(failed, 0)
}

// sweepableProviders returns, sorted, the providers in a run that ingested at least one
// job — the only ones safe to sweep (a zero-ingest provider proves only that its crawl
// failed). Sorting gives a deterministic sweep order across runs and tests.
func sweepableProviders(rs pipeline.RunStats) []string {
	providers := make([]string, 0, len(rs))
	for provider, s := range rs {
		if shouldSweep(s) {
			providers = append(providers, provider)
		}
	}
	sort.Strings(providers)
	return providers
}

// shouldSweep reports whether the run saw enough of the world to justify closing
// jobs: a run that ingested nothing proves only that the crawl failed.
func shouldSweep(stats pipeline.Stats) bool {
	return stats.Ingested > 0
}
