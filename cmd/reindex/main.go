// Command reindex rebuilds the Meilisearch jobs (facet/keyword) index from
// Postgres. It ensures the index settings exist, then scans the WHOLE table in
// batches and upserts their documents into a fresh rebuild index before atomically
// swapping it in. Run it on a schedule (e.g. cron); it processes the whole table and
// exits. Indexing is idempotent (upsert by id), so re-runs are safe. A full rebuild
// is minutes, not hours — the index carries no embedder.
package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// reindexBatchSize bounds how many jobs are read from Postgres and pushed to
// Meilisearch per round. Once the facet index dropped the per-document embedder,
// the per-batch round-trip became the throughput lever, so the batch is sized up
// from 500 to amortize it (Postgres read and the ~7KB-doc payload are both cheap
// at this size). A const for now; promote to config if it needs tuning.
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
	q := db.New(pool)

	// Guard free disk for the swap rebuild up front — BEFORE the expensive recompute —
	// so a disk refusal is a true no-op (no prod writes, no full-catalogue scans).
	rcfg := config.LoadReindex()
	if err := guardDisk(rcfg.MeiliDataDir, rcfg.MinFreeGB, statfsFree); err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}

	// The duplicate-marker passes run only when asked for (REINDEX_DEDUP=1) — see
	// config.Reindex.Dedup for why they are no longer part of every rebuild. Without
	// them the rebuild still collapses reposts; it just uses the markers the last
	// dedup invocation left, which is what "eventually consistent" already meant.
	if rcfg.Dedup {
		refreshDuplicateMarkers(ctx, q)
	}

	reader := worker.NewFullScanReader(q)
	lookup, err := buildRealityLookup(ctx, q)
	if err != nil {
		log.Printf("reindex: build reality lookup: %v", err)
		return 1
	}
	geo, err := buildClusterGeoLookup(ctx, q)
	if err != nil {
		log.Printf("reindex: build cluster geo lookup: %v", err)
		return 1
	}

	// Builds a fresh index and atomically swaps it in — transiently a second full copy
	// of the index (disk already guarded up front, before the recompute).
	log.Print("reindex: target=facet scope=full mode=swap")
	indexed, skipped, err := reindexFull(ctx, reader, client.NewFacetRebuild(), lookup, geo, time.Now())
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
func refreshDuplicateMarkers(ctx context.Context, q *db.Queries) {
	if n, err := recomputeRoleDuplicates(ctx, q); err != nil {
		log.Printf("reindex: recompute role duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: recomputed role duplicates (%d rows re-marked)", n)
	}

	if n, err := suppressAggregatorDuplicates(ctx, q); err != nil {
		log.Printf("reindex: suppress aggregator duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: suppressed aggregator duplicates (%d rows re-marked)", n)
	}

	if n, err := collapseFuzzyDuplicates(ctx, q); err != nil {
		log.Printf("reindex: collapse fuzzy duplicates (continuing with prior markers): %v", err)
	} else if n > 0 {
		log.Printf("reindex: collapsed fuzzy duplicates (%d rows re-marked)", n)
	}
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
func reindexFull(ctx context.Context, reader worker.PageReader, b rebuilder, lookup realityLookup, geo clusterGeoLookup, now time.Time) (int, int, error) {
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

	indexed, skipped, err := streamOpenDocs(ctx, reader, lookup, geo, now, b.Push)
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
func streamOpenDocs(ctx context.Context, reader worker.PageReader, lookup realityLookup, geo clusterGeoLookup, now time.Time, push func(context.Context, []search.JobDocument) error) (int, int, error) {
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
			docs, _, err := splitJobs(jobs, lookup, geo, now) // closed jobs (deleteIDs) are dropped, not indexed
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

// realityLookup returns a role cluster's repost and concurrent-open counts for the
// job-reality signal. A miss (a role not in the precomputed map, i.e. a singleton)
// yields (1, 1) — a unique, non-reposted role. A nil lookup means the counts default
// to (1, 1) everywhere (used by tests that do not exercise clustering).
type realityLookup func(companySlug, fingerprint string) (repost, mass int)

// clusterGeoLookup returns the union of a role cluster's countries, regions, and cities
// across its open rows, so the canon's search document can be widened beyond its own
// geography. A miss (singleton cluster) yields nil slices — a no-op widening. A nil
// lookup skips widening entirely (tests that do not exercise clustering).
type clusterGeoLookup func(companySlug, fingerprint string) (countries, regions, cities []string)

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

// forEachCompany runs fn once per company, summing the rows it reports re-marked.
// Companies are independent, so one failure must not starve the rest — it is counted
// and skipped, and any failure turns the pass's result into an aggregate error (the
// caller treats the whole pass as best-effort and continues with the prior markers).
// Unlike forCompanyBatches, this stays one-at-a-time: its only caller
// (collapseFuzzyDuplicates, cmd/reindex/fuzzy.go) does real per-company work in Go —
// fetching titles and descriptions, then fuzzy-comparing them in memory — not a single
// set-based SQL statement, so there is no query to batch the way the two SQL-only
// passes were.
func forEachCompany(ctx context.Context, companies []string, fn func(context.Context, string) (int64, error)) (int64, error) {
	var total int64
	var failures int
	var lastErr error
	for _, c := range companies {
		n, err := fn(ctx, c)
		if err != nil {
			failures++
			lastErr = fmt.Errorf("company %q: %w", c, err)
			continue
		}
		total += n
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

// buildClusterGeoLookup precomputes the whole-catalogue role-cluster geography union once
// (RoleClusterGeoAll returns only open multi-row clusters), so widening each canon during
// the rebuild is a map read, not N queries. A singleton cluster is absent from the map and
// resolves to nil slices — a no-op widening.
func buildClusterGeoLookup(ctx context.Context, q *db.Queries) (clusterGeoLookup, error) {
	rows, err := q.RoleClusterGeoAll(ctx)
	if err != nil {
		return nil, err
	}
	type geo struct{ countries, regions, cities []string }
	m := make(map[string]geo, len(rows))
	for _, r := range rows {
		m[r.CompanySlug+"\x00"+r.RoleFingerprint.String] = geo{r.Countries, r.Regions, r.Cities}
	}
	return func(cs, fp string) ([]string, []string, []string) {
		g := m[cs+"\x00"+fp]
		return g.countries, g.regions, g.cities
	}, nil
}

// splitJobs partitions a batch from the (deliberately unfiltered) reindex feed:
// open, non-private, categorized jobs become index documents (each carrying its
// reality signal, classified against `now` and its cluster counts); closed, private,
// or category-unresolved jobs become deletions so they leave the index (the index
// contains only open, non-private, categorized jobs — see the job-search spec).
func splitJobs(jobs []db.Job, lookup realityLookup, geo clusterGeoLookup, now time.Time) ([]search.JobDocument, []int64, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	deleteIDs := make([]int64, 0, len(jobs))
	for _, j := range jobs {
		// A closed job, a non-canonical repost (duplicate_of set), a private job (the
		// jd-tailor-intake path — visible only to its creator), or a job whose category
		// neither the title dictionary nor the LLM ever resolved (search.CategoryUnresolved)
		// leaves the index: only the open, non-private, categorized canonical row of each
		// role cluster is searchable. Deleting (not just skipping) removes a row that was
		// indexed before it was closed, demoted, marked private, or — for a job this run
		// re-evaluates fresh every time — before this exclusion existed.
		if j.ClosedAt.Valid || j.DuplicateOf.Valid || j.IsPrivate || search.CategoryUnresolved(j) {
			deleteIDs = append(deleteIDs, j.ID)
			continue
		}
		repost, mass := 1, 1
		if lookup != nil {
			repost, mass = lookup(j.CompanySlug, j.RoleFingerprint.String)
		}
		doc, err := search.FromJob(j)
		if err != nil {
			return nil, nil, err
		}
		reality := jobview.ClassifyReality(j, now, repost, mass)
		doc.Reality = &reality
		// Widen the canon's geography with its cluster's union, so a collapsed
		// multi-country role stays findable by every country its reposts hold. A miss
		// (singleton cluster or no lookup) leaves the canon's own geography untouched.
		if geo != nil {
			doc.MergeClusterGeography(geo(j.CompanySlug, j.RoleFingerprint.String))
		}
		docs = append(docs, doc)
	}
	return docs, deleteIDs, nil
}
