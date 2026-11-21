// Command reindex rebuilds the Meilisearch jobs (facet/keyword) index from
// Postgres. It ensures the index settings exist, then scans every open, canonical,
// public posting in batches and upserts their documents into a fresh rebuild index
// before atomically swapping it in. Run it on a schedule (e.g. cron); it processes the
// whole catalogue and exits. Indexing is idempotent (upsert by id), so re-runs are safe.
//
// The index carries ONE `userProvided` embedder: the skill vectors behind the match sort are computed
// here (internal/dict/skillvec), so a rebuild contacts no embedding service. Building the vector index makes
// a rebuild materially slower and larger, so schedule it deliberately rather than on a tight timer.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// reindexBatchSize bounds how many jobs are read from Postgres and pushed to Meilisearch per round; the
// per-batch round-trip is the throughput lever. A const for now; promote to config if it needs tuning.
const reindexBatchSize = 2000

// progressInterval is how often reindex emits a heartbeat with its running totals, so a slow run
// can be told from a stalled one (totals stop advancing).
const progressInterval = 60 * time.Second

// drainHoldTTL is how long the drain stays paused if this process dies without lifting it.
const drainHoldTTL = 30 * time.Minute

// lookupWorkMem is the sort and hash budget for the whole-catalogue lookups, as in the rollups:
// at the default both spill to disk for hours.
const lookupWorkMem = "256MB"

func main() {
	worker.Main(run)
}

func run() int {
	// -facetd-only rebuilds facetd (FACETD_URL) alone: no Meilisearch.
	facetdOnly := flag.Bool("facetd-only", false, "rebuild only facetd (needs FACETD_URL), skipping Meilisearch")
	fuzzyCompany := flag.String("fuzzy-company", "", "refresh fuzzy markers for one company (requires REINDEX_DEDUP_ONLY=1)")
	flag.Parse()
	rcfg := config.LoadReindex()
	if *fuzzyCompany != "" && (!rcfg.DedupOnly || *facetdOnly) {
		log.Print("config: -fuzzy-company requires REINDEX_DEDUP_ONLY=1 without -facetd-only")
		return 1
	}
	facetdURL := os.Getenv("FACETD_URL")
	if *facetdOnly && facetdURL == "" {
		log.Print("config: -facetd-only needs FACETD_URL")
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Catalogue-wide index work (full rebuild, marker passes, suggestion build) takes this same lock one at a time:
	// the schedule cannot keep them apart, and overlapping runs have taken the site down. Skip rather than wait:
	// each is idempotent and due again soon, while waiting holds a Type=oneshot unit open for hours.
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
	// How many companies the fuzzy pass works on at once. Its work waits on Postgres, not the
	// CPU, so a few in flight cut its wall time several-fold; each holds a pool connection
	// (database.defaultMaxConns is 10, one of them already the index lock's), so past that a
	// higher value only queues on the pool.
	fuzzyWorkers, err := worker.EnvInt32("REINDEX_FUZZY_WORKERS", 4)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	if *fuzzyCompany != "" {
		changed, err := collapseFuzzyDuplicatesForCompany(ctx, q, *fuzzyCompany)
		if err != nil {
			log.Printf("reindex: fuzzy company %q: %v", *fuzzyCompany, err)
			return 1
		}
		log.Printf("reindex: fuzzy company %q: %d rows re-marked", *fuzzyCompany, changed)
		return 0
	}

	// Marker-only mode stops here, before anything Meilisearch-related; see config.Reindex.DedupOnly.
	//
	// It is also the one mode whose exit code carries the passes' outcome: they are log-and-continue so a
	// failure cannot stop the rebuild after them, but here there is no rebuild to protect.
	if rcfg.DedupOnly {
		if rcfg.DedupBudget > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, rcfg.DedupBudget)
			defer cancel()
		}
		return dedupOnlyExit(ctx, q, int(fuzzyWorkers))
	}

	// Captured before anything else runs: a full facet reindex reads every job's CURRENT content, so any
	// search_outbox row queued before this instant is already reflected in the scan. Purged after a successful
	// facet-only swap (see the call site near the bottom).
	startedAt := time.Now()

	// Bootstrap owns config + pool, so this required-config check lands just after the pool opens;
	// cleanup closes it on this early return.
	if cfg.MeiliKey == "" && !*facetdOnly {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	// No WithEmbed* options: the facet rebuild never embeds anything.
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	// Guard free disk for the swap rebuild up front, BEFORE the recompute, so a refusal is a true no-op.
	if !*facetdOnly {
		if err := guardDisk(rcfg.MeiliDataDir, rcfg.MinFreeGB, statfsFree); err != nil {
			log.Printf("reindex: %v", err)
			return 1
		}
	}

	// The duplicate-marker passes run only when asked for (REINDEX_DEDUP=1; see config.Reindex.Dedup). Otherwise
	// the rebuild uses the markers the last dedup invocation left.
	if rcfg.Dedup {
		done := worker.Stage("duplicate markers")
		refreshDuplicateMarkers(ctx, q, int(fuzzyWorkers))
		done()
	}

	// Only the open, canonical, public rows: splitJobs would drop every other row anyway (see ListIndexableJobsByIDAfter).
	reader := worker.NewIndexableScanReader(q)
	done := worker.Stage("reality lookup")
	var lookup realityLookup
	err = withLookupMemory(ctx, pool, func(q *db.Queries) (err error) {
		lookup, err = buildRealityLookup(ctx, q)
		return err
	})
	if err != nil {
		log.Printf("reindex: build reality lookup: %v", err)
		return 1
	}
	done()
	done = worker.Stage("closure geo lookup")
	var geo closureGeoLookup
	err = withLookupMemory(ctx, pool, func(q *db.Queries) (err error) {
		geo, err = buildClosureGeoLookup(ctx, q)
		return err
	})
	if err != nil {
		log.Printf("reindex: build closure geo lookup: %v", err)
		return 1
	}
	done()

	// Builds a fresh index and atomically swaps it in, transiently a second full copy of the index (disk is guarded up front).
	// Only from here does the rebuild write to Meilisearch, so only from here does it compete with the drain,
	// whose pushes re-merge the whole index; everything above reads Postgres.
	releaseDrain := func() {}
	if !*facetdOnly {
		release, err := worker.HoldPause(ctx, cfg.RedisURL, worker.SearchDrainJob, drainHoldTTL)
		if err != nil {
			log.Printf("reindex: could not pause the drain, the load will compete with it: %v", err)
		} else {
			releaseDrain = release
			defer release()
		}
	}
	var rb rebuilder
	switch {
	case *facetdOnly:
		rb = newFacetdRebuild(facetdURL)
	case facetdURL != "":
		rb = &teeRebuild{primary: client.NewFacetRebuild(), secondary: newFacetdRebuild(facetdURL)}
	default:
		rb = client.NewFacetRebuild()
	}
	log.Printf("reindex: target=facet scope=full mode=swap facetd=%t facetd_only=%t", facetdURL != "", *facetdOnly)
	done = worker.Stage("index and swap")
	indexed, skipped, err := reindexFull(ctx, reader, rb, lookup, geo, q, time.Now())
	if err != nil {
		log.Printf("reindex: %v", err)
		return 1
	}
	done()
	worker.ReportWork(int64(indexed+skipped), int64(indexed))
	log.Printf("reindex done: target=facet scope=full indexed=%d skipped=%d", indexed, skipped)
	if *facetdOnly {
		return 0 // Meilisearch was not rebuilt: the outbox rows are still owed to it
	}
	if t, ok := rb.(*teeRebuild); ok && t.secondaryDown {
		// Meilisearch is done: the drain need not wait out facetd's second pass.
		releaseDrain()
		retry := func(ctx context.Context, b rebuilder) error {
			_, _, err := reindexFull(ctx, reader, b, lookup, geo, q, time.Now())
			return err
		}
		if !retryFacetd(ctx, t, retry) {
			return 0
		}
	}

	// Best-effort: the reindex already succeeded, so a purge failure just leaves those rows for the next cycle.
	if n, err := q.DeleteSearchOutboxCreatedBefore(ctx, pgconv.Instant(startedAt)); err != nil {
		log.Printf("reindex: purge stale search_outbox entries: %v", err)
	} else if n > 0 {
		log.Printf("reindex: purged %d stale search_outbox entries queued before this run", n)
	}
	return 0
}

// dedupOnlyExit is the whole of the marker-only mode: the three passes, and their outcome as an exit code.
// It is a function so a test can assert the exit code without run()'s pool, config and Meilisearch client;
// otherwise re-introducing a swallowed outcome (`refreshDuplicateMarkers(ctx, q); return 0`) would keep every test green.
func dedupOnlyExit(ctx context.Context, q *db.Queries, fuzzyWorkers int) int {
	// Riding the marker-only schedule, which runs several times a day: without it a posting
	// that only aged would reach search at a full rebuild, and those no longer run on a timer.
	// First, because it is one query and the marker passes are what the unit's timeout cuts.
	failed := 0
	if err := enqueueTimeCrossings(ctx, q, time.Now()); err != nil {
		failed++
		log.Printf("reindex: time crossings: %v", err)
	}
	failed += refreshDuplicateMarkers(ctx, q, fuzzyWorkers)
	return worker.ExitCode(failed, 0)
}

// refreshDuplicateMarkers runs the three duplicate-marker passes in dependency order: role clusters first
// (ATS reposts collapse to their canon), then aggregator suppression, then the fuzzy collapse (which only
// claims what the exact passes did not).
//
// Every pass is best-effort and logs rather than fails, so a marker hiccup cannot stop the rebuild after it.
// Each runs per company in short transactions, never a table-wide lock that would stall ingest.
//
// It returns how many passes failed. Full-rebuild callers ignore it; REINDEX_DEDUP_ONLY does not, since
// there these passes ARE the run. A pass the run's own deadline (REINDEX_DEDUP_BUDGET) stopped has not
// failed: it saved its place and the next run carries on from there.
func refreshDuplicateMarkers(ctx context.Context, q *db.Queries, fuzzyWorkers int) int {
	passes := []struct {
		failed, done string
		run          func() (int64, error)
	}{
		{"recompute role duplicates", "recomputed role duplicates", func() (int64, error) { return recomputeRoleDuplicates(ctx, q) }},
		{"suppress aggregator duplicates", "suppressed aggregator duplicates", func() (int64, error) { return suppressAggregatorDuplicates(ctx, q) }},
		{"collapse fuzzy duplicates", "collapsed fuzzy duplicates", func() (int64, error) { return collapseFuzzyDuplicates(ctx, q, fuzzyWorkers) }},
	}
	failed := 0
	for _, pass := range passes {
		n, err := pass.run()
		switch {
		case err != nil && budgetSpent(ctx, err):
			log.Printf("reindex: %s: run budget spent, the next run resumes it (%d rows re-marked)", pass.failed, n)
		case err != nil:
			failed++
			log.Printf("reindex: %s (continuing with prior markers): %v", pass.failed, err)
		case n > 0:
			log.Printf("reindex: %s (%d rows re-marked)", pass.done, n)
		}
	}
	return failed
}

// budgetSpent reports whether err is the run's own deadline and nothing else: a SIGTERM (Canceled), a
// statement timeout or a resume point that failed to save beside the deadline is still a failure.
func budgetSpent(ctx context.Context, err error) bool {
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			if !budgetSpent(ctx, e) {
				return false
			}
		}
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// rebuilder builds a brand-new index out of band and atomically swaps it into production. Prepare creates a
// fresh, empty index; Push streams batches into it WITHOUT waiting per batch (so Meilisearch auto-batches
// them); Promote waits for the pushes, swaps it over the live one atomically and drops the old one. Search
// serves the old index until the swap, and merges stay cheap because the new index grows from empty.
type rebuilder interface {
	Prepare(ctx context.Context) error
	Push(ctx context.Context, docs []search.JobDocument) error
	Promote(ctx context.Context) error
	// Cleanup drops a half-built rebuild index; reindexFull defers it so an aborted run leaves no orphan eating disk.
	Cleanup(ctx context.Context) error
}

// reindexFull rebuilds the index from scratch and swaps it in. It streams ONLY open jobs into the fresh index;
// closed jobs are simply absent. fetch pages by keyset (id > last seen) so rows inserted or re-ordered
// during the run cannot be skipped or repeated.
func reindexFull(ctx context.Context, reader worker.PageReader, b rebuilder, lookup realityLookup, geo closureGeoLookup, forms applyform.FormLister, now time.Time) (int, int, error) {
	if err := b.Prepare(ctx); err != nil {
		return 0, 0, err
	}

	// Promote ends the happy path by swapping and dropping the old index; any earlier return is an abort that
	// leaves the half-built rebuild index behind. Drop it in a defer, best-effort on a cancellation-immune
	// context so it still reaches Meilisearch when the abort was the parent ctx being cancelled.
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

	indexed, skipped, err := streamOpenDocs(ctx, reader, lookup, geo, forms, now, b.Push)
	if err != nil {
		return indexed, skipped, err
	}

	if err := b.Promote(ctx); err != nil {
		return indexed, skipped, err
	}
	promoted = true
	return indexed, skipped, nil
}

// streamOpenDocs pages the reindex feed by keyset and pushes each batch's open-job documents through push,
// returning the running indexed/skipped totals.
//
// forms supplies each batch's application-form question counts in one query per batch;
// nil writes none.
func streamOpenDocs(ctx context.Context, reader worker.PageReader, lookup realityLookup, geo closureGeoLookup, forms applyform.FormLister, now time.Time, push func(context.Context, []search.JobDocument) error) (int, int, error) {
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
			// Asked for the whole page, including jobs splitJobs dropped, to avoid deciding indexability twice.
			// A job with no captured form is absent from the map: its document carries no count (unknown, not short).
			questions, err := applyform.QuestionCounts(ctx, forms, pageIDs(jobs))
			if err != nil {
				return int(indexed.Load()), skipped, err
			}
			for i := range docs {
				docs[i].AttachApplyQuestions(questions)
			}
			if err := push(ctx, docs); err != nil {
				return int(indexed.Load()), skipped, err
			}
			indexed.Add(int64(len(docs)))
		}

		// Keyset progress is the exhaustion signal: ResilientPage advances lastID past a skipped (corrupted)
		// row, so a short page from the degrade path does not end the scan early.
		if lastID == afterID {
			break
		}
		afterID = lastID
	}
	return int(indexed.Load()), skipped, nil
}

// pageIDs is the ids of one page of the reindex feed.
func pageIDs(jobs []db.Job) []int64 {
	ids := make([]int64, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	return ids
}

// realityLookup returns a role cluster's repost and concurrent-open counts for the
// job-reality signal. A miss (a role not in the precomputed map, i.e. a singleton)
// yields (1, 1) — a unique, non-reposted role. A nil lookup means the counts default
// to (1, 1) everywhere (used by tests that do not exercise clustering).
type realityLookup func(companySlug, fingerprint string) (repost, mass int)

// closureGeoLookup returns the union of countries, regions, cities, and metros across every open row a
// searchable row REPRESENTS (its duplicate closure), so its document can be widened beyond its own geography.
// Keyed by job id, not (company_slug, role_fingerprint): the dedup passes that suppress rows leave them with
// fingerprints their canon does not share.
//
// A miss (the row represents nobody) yields a zero ClosureGeo — a no-op widening. A nil lookup skips
// widening entirely (tests that do not exercise clustering).
type closureGeoLookup func(ownerID int64) jobview.ClosureGeo

// withLookupMemory runs read in a transaction with the lookups' sort and hash budget.
func withLookupMemory(ctx context.Context, pool *pgxpool.Pool, read func(*db.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL work_mem = '"+lookupWorkMem+"'"); err != nil {
		return err
	}
	if err := read(db.New(pool).WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// buildRealityLookup precomputes the whole-catalogue role-cluster counts once, so classification is a map read.
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
// (DuplicateClosureGeoAll returns only rows representing at least one other open row). A row representing
// nobody is absent and resolves to a zero ClosureGeo, a no-op widening.
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

// splitJobs partitions a batch from the reindex feed: open, non-private jobs search does not withhold become
// index documents (each carrying its reality signal); closed, private, or withheld jobs become deletions.
func splitJobs(jobs []db.Job, lookup realityLookup, geo closureGeoLookup, now time.Time) ([]search.JobDocument, []int64, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	deleteIDs := make([]int64, 0, len(jobs))
	for _, j := range jobs {
		// A closed job, a non-canonical repost (duplicate_of set), a private job (visible only to its creator), or one
		// search withholds (search.Withheld) leaves the index. Delete rather than skip, so a row indexed before it
		// was closed, demoted, made private or went unseen is removed.
		if j.ClosedAt.Valid || j.DuplicateOf.Valid || j.IsPrivate || search.Withheld(j, now, unseenWindows) {
			deleteIDs = append(deleteIDs, j.ID)
			continue
		}
		repost, mass := 1, 1
		if lookup != nil {
			repost, mass = lookup(j.CompanySlug, j.RoleFingerprint.String)
		}
		doc, err := search.FromJob(j, sourceTypeOf(j.Source))
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
