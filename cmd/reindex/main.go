// Command reindex rebuilds the Meilisearch jobs (facet/keyword) index from
// Postgres. It ensures the index settings exist, then scans every open, canonical,
// public posting in batches and upserts their documents into a fresh rebuild index
// before atomically swapping it in. Run it on a schedule (e.g. cron); it processes the
// whole catalogue and exits. Indexing is idempotent (upsert by id), so re-runs are safe.
//
// The index carries ONE embedder, and it is `userProvided`: the skill vectors backing
// the match sort are computed here (internal/dict/skillvec) rather than fetched, so a
// rebuild still contacts no embedding service. It is not free, though — building the
// vector index is what makes a rebuild materially slower and larger than it was when
// the index carried no embedder at all, so this is a run to schedule deliberately
// rather than one to leave on a tight timer.
package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// reindexBatchSize bounds how many jobs are read from Postgres and pushed to
// Meilisearch per round. When the facet index dropped its model-backed embedder, the
// per-batch round-trip became the throughput lever, so the batch was sized up from
// 500 to amortize it (Postgres read and the ~7KB-doc payload are both cheap at this
// size). The skill vectors added since do not change that: they are computed locally
// and add bytes to the payload, not a round trip. A const for now; promote to config
// if it needs tuning.
const reindexBatchSize = 2000

// progressInterval is how often reindex emits a heartbeat with its running totals.
// A full reindex pushes hundreds of thousands of docs to Meilisearch and otherwise
// logs only on completion, so the heartbeat distinguishes a slow run from a stalled
// one (the totals stop advancing).
const progressInterval = 60 * time.Second

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

	// Catalogue-wide index work runs one at a time — full rebuild, marker passes and the
	// suggestion build all take this same lock. The schedule cannot keep them apart on its
	// own: a rebuild took 4.5 hours on 2026-09-15 against clock times laid out when it took
	// three, so the 06:45, 16:30 and 19:30 jobs land inside one every day. That afternoon
	// two of them together took the site down. Skipping is right rather than waiting: every
	// one of these is idempotent and due again soon, while waiting holds a Type=oneshot unit
	// open for hours and reads as a hang.
	releaseLock, gotLock, err := worker.HoldHeavyIndexLock(ctx, pool)
	if err != nil {
		log.Printf("index lock: %v", err)
		return 1
	}
	if !gotLock {
		log.Print("reindex: another catalogue-wide index job is running — skipping this run")
		return 0
	}
	defer releaseLock()

	q := db.New(pool)
	rcfg := config.LoadReindex()
	// How many companies the fuzzy pass works on at once. Its work waits on Postgres, not the
	// CPU, so a few in flight cut its wall time several-fold; each holds a pool connection
	// (database.defaultMaxConns is 10, one of them already the index lock's), so past that a
	// higher value only queues on the pool.
	fuzzyWorkers, err := worker.EnvInt32("REINDEX_FUZZY_WORKERS", 4)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// Marker-only mode stops here, before anything Meilisearch-related — see
	// config.Reindex.DedupOnly for why this can run far more often than a full
	// Dedup+rebuild invocation.
	//
	// It is also the one mode whose exit code can carry the passes' outcome. They are
	// log-and-continue because "a marker refresh failure must not stop the rebuild that
	// follows it" — and here there is no rebuild to protect, so a run in which all three
	// passes failed used to exit 0, indistinguishable from one in which the catalogue
	// simply had nothing to re-mark. The cancellation line forCompanyBatches assembles
	// was thrown away with it.
	if rcfg.DedupOnly {
		return dedupOnlyExit(ctx, q, int(fuzzyWorkers))
	}

	// Captured before anything else runs (including the duplicate-marker recompute
	// passes below, which can themselves take minutes under load): a full facet
	// reindex reads every job's CURRENT content, so any search_outbox row queued
	// before this instant is provably already reflected in what the scan reads.
	// Purged after a successful facet-only swap — see the call site near the bottom.
	startedAt := time.Now()

	// Bootstrap owns config + pool, so this required-config check lands just after
	// the pool opens rather than before it. The connect is cheap and cleanup closes
	// it on this early return, so the only cost of a missing key is one DB handshake.
	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	// No WithEmbed* options: the facet rebuild never embeds anything (that was only ever
	// true of the removed --semantic pass), so there is nothing here for EMBED_URL/
	// EMBED_API_KEY/EMBED_CONCURRENCY to configure.
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	// Guard free disk for the swap rebuild up front — BEFORE the expensive recompute —
	// so a disk refusal is a true no-op (no prod writes, no full-catalogue scans).
	if err := guardDisk(rcfg.MeiliDataDir, rcfg.MinFreeGB, statfsFree); err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}

	// The duplicate-marker passes run only when asked for (REINDEX_DEDUP=1) — see
	// config.Reindex.Dedup for why they are no longer part of every rebuild. Without
	// them the rebuild still collapses reposts; it just uses the markers the last
	// dedup invocation left, which is what "eventually consistent" already meant.
	if rcfg.Dedup {
		refreshDuplicateMarkers(ctx, q, int(fuzzyWorkers))
	}

	// Only the open, canonical, public rows: splitJobs would drop every other row anyway,
	// and reading them was most of the rebuild's cost (see ListIndexableJobsByIDAfter).
	reader := worker.NewIndexableScanReader(q)
	lookup, err := buildRealityLookup(ctx, q)
	if err != nil {
		log.Printf("reindex: build reality lookup: %v", err)
		return 1
	}
	geo, err := buildClosureGeoLookup(ctx, q)
	if err != nil {
		log.Printf("reindex: build closure geo lookup: %v", err)
		return 1
	}

	// Builds a fresh index and atomically swaps it in — transiently a second full copy
	// of the index (disk already guarded up front, before the recompute).
	log.Print("reindex: target=facet scope=full mode=swap")
	indexed, skipped, err := reindexFull(ctx, reader, client.NewFacetRebuild(), lookup, geo, q, time.Now())
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	log.Printf("reindex done: target=facet scope=full indexed=%d skipped=%d", indexed, skipped)

	// Best-effort: the reindex itself already succeeded, so a purge failure just leaves
	// those rows for the next cycle rather than failing this run.
	if n, err := q.DeleteSearchOutboxCreatedBefore(ctx, pgtype.Timestamptz{Time: startedAt, Valid: true}); err != nil {
		log.Printf("reindex: purge stale search_outbox entries: %v", err)
	} else if n > 0 {
		log.Printf("reindex: purged %d stale search_outbox entries queued before this run", n)
	}
	return 0
}

// refreshDuplicateMarkers runs the three duplicate-marker passes in the order they
// depend on: role clusters first (so ATS reposts collapse to their canon), then
// aggregator suppression (so an aggregator copy of an already-collapsed ATS posting
// drops out), then the fuzzy collapse (which only ever claims what the exact passes
// did not).
//
// Every pass is best-effort and logs rather than fails: a hiccup in a marker refresh
// must not stop the rebuild that follows it, which also owns index settings and
// compaction. Each is done per company in short transactions — never a table-wide
// lock that would stall the ingest.
//
// It returns how many of the three passes failed, so a caller with no rebuild behind it
// can still say so in its exit code. The full-rebuild callers ignore it, which is the
// best-effort rule above, unchanged; REINDEX_DEDUP_ONLY does not, because in that mode
// these three passes ARE the run.
// dedupOnlyExit is the whole of the marker-only mode: the three passes, and their outcome
// as an exit code.
//
// It is a function rather than two lines inside run() so that the second half can be
// tested at all. run() needs a pool, a config and a Meilisearch client before it reaches
// the branch, so a test asserting on the exit code has to go through this seam — and
// without one, re-introducing the swallowed outcome (`refreshDuplicateMarkers(ctx, q);
// return 0`) leaves every test in this package green, which is how it shipped that way
// the first time.
func dedupOnlyExit(ctx context.Context, q *db.Queries, fuzzyWorkers int) int {
	return worker.ExitCode(refreshDuplicateMarkers(ctx, q, fuzzyWorkers), 0)
}

func refreshDuplicateMarkers(ctx context.Context, q *db.Queries, fuzzyWorkers int) int {
	failed := 0

	if n, err := recomputeRoleDuplicates(ctx, q); err != nil {
		failed++
		log.Printf("reindex: recompute role duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: recomputed role duplicates (%d rows re-marked)", n)
	}

	if n, err := suppressAggregatorDuplicates(ctx, q); err != nil {
		failed++
		log.Printf("reindex: suppress aggregator duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: suppressed aggregator duplicates (%d rows re-marked)", n)
	}

	if n, err := collapseFuzzyDuplicates(ctx, q, fuzzyWorkers); err != nil {
		failed++
		log.Printf("reindex: collapse fuzzy duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: collapsed fuzzy duplicates (%d rows re-marked)", n)
	}

	return failed
}

// rebuilder builds a brand-new index out of band and atomically swaps it into
// production. A full reindex uses it instead of mutating the live index in place:
// Prepare creates a fresh, empty rebuild index; Push streams document batches into
// it WITHOUT waiting per batch (so Meilisearch auto-batches them — the throughput
// lever); Promote waits for the pushes to finish, swaps the rebuild index over the
// live one in a single atomic step, and drops the old one. Search keeps serving the
// old index untouched until the swap, and merges stay cheap because the rebuild
// index grows from empty rather than re-merging into a full one.
type rebuilder interface {
	Prepare(ctx context.Context) error
	Push(ctx context.Context, docs []search.JobDocument) error
	Promote(ctx context.Context) error
	// Cleanup drops a half-built rebuild index. reindexFull defers it so a run that
	// aborts before Promote's swap-and-drop does not leave an orphan index eating disk.
	Cleanup(ctx context.Context) error
}

// reindexFull rebuilds the index from scratch and swaps it in. It streams ONLY
// open jobs into the fresh index — closed jobs are simply absent (the rebuild
// index never held them, so unlike the in-place path there is nothing to delete).
// fetch pages by keyset (id > last seen) so rows inserted or re-ordered during the
// run cannot be skipped or repeated.
func reindexFull(ctx context.Context, reader worker.PageReader, b rebuilder, lookup realityLookup, geo closureGeoLookup, chunks search.ChunkLister, now time.Time) (int, int, error) {
	if err := b.Prepare(ctx); err != nil {
		return 0, 0, err
	}

	// Promote ends the happy path by swapping the rebuild index in and dropping the old
	// one; any earlier return is an abort that leaves the half-built rebuild index behind.
	// Drop it in a defer so an aborted run never orphans an index that eats disk until the
	// next run's Prepare clears it. Best-effort on a cancellation-immune context so it can
	// still reach Meilisearch when the abort was the parent ctx being cancelled.
	promoted := false
	defer func() {
		if promoted {
			return
		}
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := b.Cleanup(cctx); err != nil {
			log.Printf("reindex: cleanup aborted rebuild index (best-effort): %v", err)
		}
	}()

	indexed, skipped, err := streamOpenDocs(ctx, reader, lookup, geo, chunks, now, b.Push)
	if err != nil {
		return indexed, skipped, err
	}

	if err := b.Promote(ctx); err != nil {
		return indexed, skipped, err
	}
	promoted = true
	return indexed, skipped, nil
}

// streamOpenDocs pages the reindex feed by keyset and pushes each batch's open-job
// documents through push, returning the running indexed/skipped totals. Its own
// function (rather than inlined into reindexFull) mostly for historical reasons — it
// used to also back an in-place semantic rehydration path that shared this loop and
// differed only in the per-batch sink; that path is gone (see openspec/changes/
// drop-hybrid-search-pgvector-similar), so reindexFull is its only caller now.
//
// chunks supplies each batch's description vectors in one query per batch; nil (tests
// that do not exercise vectors) writes every document with none.
func streamOpenDocs(ctx context.Context, reader worker.PageReader, lookup realityLookup, geo closureGeoLookup, chunks search.ChunkLister, now time.Time, push func(context.Context, []search.JobDocument) error) (int, int, error) {
	var indexed atomic.Int64
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("reindex: progress indexed=%d", indexed.Load())
	})
	defer stopHeartbeat()

	var afterID int64
	var skipped int
	for {
		jobs, lastID, corrupted, err := worker.ResilientPage(ctx, reader, afterID, reindexBatchSize)
		if err != nil {
			return int(indexed.Load()), skipped, err
		}
		skipped += len(corrupted)

		if len(jobs) > 0 {
			desc, err := batchChunkVectors(ctx, chunks, jobs)
			if err != nil {
				return int(indexed.Load()), skipped, err
			}
			docs, _, err := splitJobs(jobs, lookup, geo, desc, now) // closed jobs (deleteIDs) are dropped, not indexed
			if err != nil {
				return int(indexed.Load()), skipped, err
			}
			if err := push(ctx, docs); err != nil {
				return int(indexed.Load()), skipped, err
			}
			indexed.Add(int64(len(docs)))
		}

		// Keyset progress is the exhaustion signal: ResilientPage advances lastID
		// past a skipped (corrupted) row, so a short page from the degrade path does
		// not end the scan early the way a "< batchSize" check would.
		if lastID == afterID {
			break
		}
		afterID = lastID
	}
	return int(indexed.Load()), skipped, nil
}

// batchChunkVectors reads one page's description vectors. Every id in the page is asked
// for, including the ones splitJobs will still drop (an unresolved category, no visible
// description): filtering first would mean deciding indexability twice.
func batchChunkVectors(ctx context.Context, chunks search.ChunkLister, jobs []db.Job) (map[int64][][]float32, error) {
	if chunks == nil {
		return nil, nil
	}
	ids := make([]int64, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	return search.ChunkVectorsForJobs(ctx, chunks, ids)
}

// realityLookup returns a role cluster's repost and concurrent-open counts for the
// job-reality signal. A miss (a role not in the precomputed map, i.e. a singleton)
// yields (1, 1) — a unique, non-reposted role. A nil lookup means the counts default
// to (1, 1) everywhere (used by tests that do not exercise clustering).
type realityLookup func(companySlug, fingerprint string) (repost, mass int)

// closureGeoLookup returns the union of countries, regions, cities, and metros across every open row
// a searchable row REPRESENTS — its duplicate closure — so the row's search document can be
// widened beyond its own geography. Keyed by job id, not by (company_slug, role_fingerprint):
// the two dedup passes that suppress rows leave them with fingerprints their canon does not
// share, so a fingerprint key could never reach them (issue #2225).
//
// A miss (the row represents nobody) yields a zero ClosureGeo — a no-op widening. A nil lookup skips
// widening entirely (tests that do not exercise clustering).
type closureGeoLookup func(ownerID int64) jobview.ClosureGeo

// companyBatchSize bounds how many companies one RecomputeRoleDuplicatesForCompanies /
// SuppressAggregatorDuplicatesForCompanies call covers. Measured on prod (2026-08-06):
// one round trip per company — 236,923 distinct open companies, 94,410 of them with an
// open aggregator posting — made the aggregator-suppression pass alone run for hours
// under ordinary host load and get stuck (systemd auto-restarted it, redoing the same
// hours of work) with the reindex never reaching the point of actually pushing to
// Meili. 500 keeps each statement's WHERE ... = ANY(...) small enough to stay a cheap
// index probe (see migration 0076's functional index) while cutting round trips by
// ~500x; it is not tuned beyond "clearly fixes the incident," so revisit if a future
// measurement suggests a better number.
const companyBatchSize = 500

// recomputeRoleDuplicates refreshes jobs.duplicate_of in batches of companies,
// returning the total rows re-marked. Batching (not one UPDATE per company, and not one
// UPDATE for the whole catalogue) balances two costs: an unbatched per-company call
// pays a full network/planning round trip per company (see companyBatchSize), while a
// single whole-catalogue UPDATE would hold its lock window across the entire jobs
// table instead of one bounded slice at a time, risking a stall of concurrent ingest
// writes. Best-effort like every batch here (see forCompanyBatches).
func recomputeRoleDuplicates(ctx context.Context, q *db.Queries) (int64, error) {
	companies, err := q.CompaniesWithRoleClusters(ctx)
	if err != nil {
		return 0, err
	}
	return forCompanyBatches(ctx, companies, q.RecomputeRoleDuplicatesForCompanies)
}

// suppressAggregatorDuplicates marks each open aggregator posting that duplicates a
// first-party ATS posting (same company, normalized title, compatible country) as a
// duplicate of that ATS row, processed in batches of companies. Returns the total rows
// re-marked. The aggregator set comes from the taxonomy registry's aggregator()
// markers, so it is the same set here as on the ingest host: a keyed adapter whose
// credential lives only where the crawl runs must still be classified, or its copies of
// an ATS posting go unsuppressed. Best-effort and lock-scoped exactly like
// recomputeRoleDuplicates.
func suppressAggregatorDuplicates(ctx context.Context, q *db.Queries) (int64, error) {
	aggregators := sources.AggregatorProviders(sources.Taxonomy())
	companies, err := q.CompaniesWithAggregatorPostings(ctx, aggregators)
	if err != nil {
		return 0, err
	}
	return forCompanyBatches(ctx, companies, func(ctx context.Context, batch []string) (int64, error) {
		return q.SuppressAggregatorDuplicatesForCompanies(ctx, db.SuppressAggregatorDuplicatesForCompaniesParams{
			FoldedCompanies: foldCompanySlugs(batch),
			Aggregators:     aggregators,
		})
	})
}

// foldCompanySlugs applies the `replace(slug, '-', ”)` fold the aggregator
// suppression compares on, so the query receives an already-folded array instead of
// folding a subquery itself.
//
// It exists for the planner, not for correctness. Folding inside the SQL meant the
// driving predicate read `= ANY(SELECT replace(c,'-',”) FROM unnest($1))`, and a
// subquery carries no size estimate — the planner assumed 200 rows and drove each
// batch off the source index, scanning ~927k aggregator rows per batch of 500
// companies (271s each on prod, ~23h for the pass, against a 12h unit timeout it
// never survived). As a bare array parameter the same batch takes 0.65s.
//
// Duplicates are left in: a fold can collide ("cfo-insights" and "cfoinsights" both
// fold to "cfoinsights"), and that collision is the POINT — those rows must match.
// Deduplicating here would change nothing for the query and cost an allocation.
func foldCompanySlugs(slugs []string) []string {
	folded := make([]string, len(slugs))
	for i, s := range slugs {
		folded[i] = strings.ReplaceAll(s, "-", "")
	}
	return folded
}

// forCompanyBatches runs fn once per companyBatchSize-sized slice of companies, summing
// the rows it reports re-marked. Batches are independent, so one failure (e.g. a
// statement timeout on an unusually large batch) must not starve the rest — it is
// counted and skipped, and any failure turns the pass's result into an aggregate error
// (the caller treats the whole pass as best-effort and continues with the prior
// markers). This is coarser fault isolation than the one-call-per-company version it
// replaced (a bad batch now costs up to companyBatchSize companies their update, not
// just one), a deliberate trade against the hours a full per-company loop cost at
// catalogue scale — see companyBatchSize.
func forCompanyBatches(ctx context.Context, companies []string, fn func(context.Context, []string) (int64, error)) (int64, error) {
	var total int64
	var done, failures int
	var lastErr error
	for batch := range slices.Chunk(companies, companyBatchSize) {
		n, err := fn(ctx, batch)
		if err != nil {
			// A cancelled context ends the pass instead of counting a failure: the
			// remaining batches would each fail instantly against the same dead
			// context, so continuing turns one deadline into hundreds of "failed"
			// batches. That is not cosmetic — it is what the 2026-08-16 investigation
			// had to see through: the log said "75 batches failed", which reads as 75
			// distinct problems rather than one timeout.
			// `done`, not `failures`: this branch runs BEFORE the failure is counted,
			// and what a cancellation needs to report is how far the pass got, not how
			// many batches were already broken.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return total, fmt.Errorf("cancelled after %d completed batches: %w", done, ctxErr)
			}
			failures++
			lastErr = fmt.Errorf("batch of %d companies (starting %q): %w", len(batch), batch[0], err)
			continue
		}
		done++
		total += n
	}
	if failures > 0 {
		return total, fmt.Errorf("%d batches failed out of %d companies (batch size %d); last: %w",
			failures, len(companies), companyBatchSize, lastErr)
	}
	return total, nil
}

// checkpointEvery is how many companies the finished prefix of a forEachCompany walk must
// grow by before it is checkpointed again. The run is ended by its unit's SIGTERM, so
// progress has to be saved along the way; at the pass's measured pace this is a write every
// minute or two, which is nothing next to the per-company work.
const checkpointEvery = 1000

// forEachCompany runs fn once per company on `workers` goroutines, summing the rows it
// reports re-marked. Unlike forCompanyBatches there is no query to batch: its only caller
// (collapseFuzzyDuplicates, cmd/reindex/fuzzy.go) does real per-company work in Go —
// fetching titles and descriptions, then fuzzy-comparing them in memory. That work waits
// on Postgres far more than it computes (measured 2026-10-28: under seven minutes of CPU
// in a four-hour run), which is why it runs several companies at once. Each company's
// marker write touches only that company's rows, so the workers never contend.
//
// Companies are independent, so one failure must not starve the rest — it is counted and
// stepped over, and any failure turns the pass's result into an aggregate error (the
// caller treats the whole pass as best-effort and continues with the prior markers).
//
// A cancelled context ends the walk instead: a company cut off by it is UNFINISHED, not
// failed, and nothing further is dispatched — otherwise one deadline reads as hundreds of
// thousands of failures, which is what the 2026-10-28 log said.
//
// checkpoint receives the resume point: the last company of the longest FINISHED prefix of
// companies (a stepped-over failure counts as finished — retrying it is the next cycle's
// job, and a company that always fails must not pin the walk). Never the newest company
// finished: with several in flight a later one finishes while an earlier one is still
// running, and resuming after the later one would skip the earlier one for good — the
// freehire#2869 trap. It is called from this goroutine only, every checkpointEvery
// companies of progress and once more at the end if the prefix moved since.
func forEachCompany(ctx context.Context, companies []string, workers int, fn func(context.Context, string) (int64, error), checkpoint func(lastFinished string)) (int64, error) {
	type result struct {
		i   int
		n   int64
		err error
	}
	indices := make(chan int)
	results := make(chan result)

	go func() {
		defer close(indices)
		for i := range companies {
			select {
			case indices <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for range min(workers, len(companies)) {
		wg.Go(func() {
			for i := range indices {
				n, err := fn(ctx, companies[i])
				results <- result{i: i, n: n, err: err}
			}
		})
	}
	go func() { wg.Wait(); close(results) }()

	var total int64
	var finished, failures int
	var lastErr error
	done := make([]bool, len(companies))
	prefix, saved := 0, 0 // companies[:prefix] are all finished; companies[:saved] were checkpointed
	for r := range results {
		if r.err != nil && ctx.Err() != nil {
			continue // cut off by the cancellation: unfinished, so the resume point stays before it
		}
		done[r.i] = true
		finished++
		if r.err != nil {
			failures++
			lastErr = fmt.Errorf("company %q: %w", companies[r.i], r.err)
		} else {
			total += r.n
		}
		for prefix < len(done) && done[prefix] {
			prefix++
		}
		if prefix-saved >= checkpointEvery {
			checkpoint(companies[prefix-1])
			saved = prefix
		}
	}
	if prefix > saved {
		checkpoint(companies[prefix-1])
	}

	if err := ctx.Err(); err != nil {
		return total, fmt.Errorf("cancelled after %d/%d companies (%d failed): %w", finished, len(companies), failures, err)
	}
	if failures > 0 {
		return total, fmt.Errorf("%d/%d companies failed; last: %w", failures, len(companies), lastErr)
	}
	return total, nil
}

// buildRealityLookup precomputes the whole-catalogue role-cluster counts once, so the
// per-job classification during the rebuild is a map read, not N queries.
func buildRealityLookup(ctx context.Context, q *db.Queries) (realityLookup, error) {
	rows, err := q.RoleClusterCountsAll(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string][2]int, len(rows))
	for _, r := range rows {
		m[r.CompanySlug+"\x00"+r.RoleFingerprint.String] = [2]int{int(r.RepostCount), int(r.MassCount)}
	}
	return func(cs, fp string) (int, int) {
		if v, ok := m[cs+"\x00"+fp]; ok {
			return v[0], v[1]
		}
		return 1, 1
	}, nil
}

// buildClosureGeoLookup precomputes the whole-catalogue duplicate-closure geography union once
// (DuplicateClosureGeoAll returns only rows that represent at least one other open row), so
// widening each document during the rebuild is a map read, not N queries. A row representing
// nobody is absent from the map and resolves to a zero ClosureGeo — a no-op widening.
func buildClosureGeoLookup(ctx context.Context, q *db.Queries) (closureGeoLookup, error) {
	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]jobview.ClosureGeo, len(rows))
	for _, r := range rows {
		m[r.OwnerID] = jobview.ClosureGeo{Countries: r.Countries, Regions: r.Regions, Cities: r.Cities, Metros: r.Metros}
	}
	return func(ownerID int64) jobview.ClosureGeo { return m[ownerID] }, nil
}

// sourceTypeOf is each source's `source_type` document attribute, resolved once from the
// adapter taxonomy (which needs no credential, so every host answers alike).
var sourceTypeOf = sources.SourceTypes(sources.Taxonomy())

// unseenWindows is how long each crawled source's postings may go unseen before they leave
// the index (search.Withheld), resolved once from the same adapter taxonomy.
var unseenWindows = search.UnseenWindows(sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow))

// splitJobs partitions a batch from the reindex feed:
// open, non-private jobs search does not withhold become index documents (each
// carrying its reality signal, classified against `now` and its cluster counts);
// closed, private, or withheld jobs become deletions so they leave the index (see
// search.Withheld and the job-search spec).
func splitJobs(jobs []db.Job, lookup realityLookup, geo closureGeoLookup, desc map[int64][][]float32, now time.Time) ([]search.JobDocument, []int64, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	deleteIDs := make([]int64, 0, len(jobs))
	for _, j := range jobs {
		// A closed job, a non-canonical repost (duplicate_of set), a private job (the
		// jd-tailor-intake path — visible only to its creator), or one search withholds —
		// no resolved category, no posting body, or a source that stopped showing it
		// (search.Withheld) — leaves the index: only the open, non-private, readable,
		// still-listed canonical row of each role cluster is searchable. Deleting (not
		// just skipping) removes a row that was indexed before it was closed, demoted,
		// marked private, or — for a job this run re-evaluates fresh every time — before
		// it went unseen or this exclusion existed.
		if j.ClosedAt.Valid || j.DuplicateOf.Valid || j.IsPrivate || search.Withheld(j, now, unseenWindows) {
			deleteIDs = append(deleteIDs, j.ID)
			continue
		}
		repost, mass := 1, 1
		if lookup != nil {
			repost, mass = lookup(j.CompanySlug, j.RoleFingerprint.String)
		}
		doc, err := search.FromJob(j, desc[j.ID], sourceTypeOf(j.Source))
		if err != nil {
			return nil, nil, err
		}
		reality := jobview.ClassifyReality(j, now, repost, mass)
		doc.Reality = &reality
		// Widen the row's geography with the union across its duplicate closure, so a
		// collapsed multi-country role stays findable by every country the rows it hides
		// are open in. A miss (the row represents nobody, or no lookup) leaves its own
		// geography untouched.
		if geo != nil {
			doc.MergeClosureGeography(geo(j.ID))
		}
		docs = append(docs, doc)
	}
	return docs, deleteIDs, nil
}
