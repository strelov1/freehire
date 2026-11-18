// Command ingest is the standalone source-ingest worker. It takes a provider name
// (passed as the first argument or via INGEST_PROVIDER), loads that provider's
// pending/active boards from the boards table, fetches each through its adapter,
// normalizes the postings, and upserts them — enqueuing new ones for enrichment in the
// same write. After the run the provider (if it ingested at least one job) has its stale
// jobs swept. Run on a schedule (e.g. cron); it processes its boards once and exits.
//
// A board's insert-time validation (provider registered, board id present unless
// boardless) already happened when it entered the boards table — see
// internal/ingest/boardcatalog — so this worker does not re-validate before crawling.
//
// Each crawl's new or content-changed jobs are queued (search_outbox, atomically with their
// write) for the live facet search index; cmd/search-drain pushes the documents on its own
// schedule, which batches many small index merges. The batch reindex stays the source of truth.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// staleAfter is the DEFAULT grace window; see sweeppolicy.StaleAfter.
const staleAfter = sweeppolicy.StaleAfter

func main() {
	worker.Main(run)
}

func run() int {
	// The unit's timeout is measured from its start, as close to here as this process can
	// observe, so the crawl deadline is too.
	started := time.Now()

	// The provider to crawl: first positional argument or INGEST_PROVIDER. An optional
	// --shard=i/n (or SHARD env) crawls a round-robin slice of its boards, spreading a
	// provider too big for one timeout (workday) over several runs. --board=<id> (optionally
	// narrowed by --region=<code>) is the operator's hand-run: resync one company/keyword
	// without disturbing the rest of the crawl.
	var provider, shardSpec, boardFilter, regionFilter string
	var reconcileLinkedIn bool
	var reconcileAfter int64
	for _, a := range os.Args[1:] {
		switch {
		case a == "--reconcile-linkedin":
			reconcileLinkedIn = true
		case strings.HasPrefix(a, "--reconcile-after="):
			var err error
			reconcileAfter, err = strconv.ParseInt(strings.TrimPrefix(a, "--reconcile-after="), 10, 64)
			if err != nil || reconcileAfter < 0 {
				log.Print("config: --reconcile-after needs a non-negative posting id")
				return 2
			}
		case strings.HasPrefix(a, "--shard="):
			shardSpec = strings.TrimPrefix(a, "--shard=")
		case a == "--shard":
			// The value must be attached (--shard=i/n); a space-separated form would swallow
			// the provider name as the selector's value.
			log.Print("config: --shard needs an attached value, e.g. --shard=2/6")
			return 1
		case strings.HasPrefix(a, "--board="):
			boardFilter = strings.TrimPrefix(a, "--board=")
		case a == "--board":
			log.Print(`config: --board needs an attached value, e.g. --board="Frontend Developer"`)
			return 1
		case strings.HasPrefix(a, "--region="):
			regionFilter = strings.TrimPrefix(a, "--region=")
		case a == "--region":
			log.Print("config: --region needs an attached value, e.g. --region=IT")
			return 1
		case a != "" && !strings.HasPrefix(a, "-") && provider == "":
			provider = a
		}
	}
	if provider == "" {
		provider = os.Getenv("INGEST_PROVIDER")
	}
	if shardSpec == "" {
		shardSpec = os.Getenv("SHARD")
	}
	if provider == "" {
		log.Print("config: no provider given (pass it as an argument or set INGEST_PROVIDER)")
		return 1
	}
	if (reconcileLinkedIn && (provider != "linkedin" || shardSpec != "" || boardFilter != "" || regionFilter != "")) || (!reconcileLinkedIn && reconcileAfter != 0) {
		log.Print("config: ownership reconciliation requires linkedin without board, region or shard filters")
		return 2
	}

	registry := sources.All(sources.NewClient())
	// Route IP-blocklisted providers through SOURCES_PROXY_URL when set; a set-but-invalid
	// value fails the run here, before the DB is touched.
	if err := sources.ApplyProxyEgress(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	// LAST, and the order is load-bearing: the Scrapling tier takes its providers off the proxy
	// and fingerprint transports wired above (see ApplyScraplingEgress). No-op without
	// SCRAPLING_FETCH_URL.
	if err := sources.ApplyScraplingEgress(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// Resolved before the DB is touched: a bad value must stop the run, not start an ordinary
	// crawl where a repair was intended.
	hydrationWindow, err := hydrationRetryWindowFor(os.Getenv("HYDRATION_RETRY_DAYS"))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if hydrationWindow != pipeline.HydrationRetryWindow {
		log.Printf("ingest: hydration retry window widened to %v — body-less rows will be re-fetched",
			hydrationWindow)
	}
	refetchAll, err := refetchAllFor(os.Getenv("INGEST_REFETCH_ALL"))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if refetchAll {
		log.Printf("ingest: INGEST_REFETCH_ALL — every listed posting is treated as new, so stored rows are re-written, not just refreshed")
	}
	recentWindow, err := recentWindowFor(os.Getenv("INGEST_RECENT_DAYS"))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if recentWindow > 0 {
		log.Printf("ingest: INGEST_RECENT_DAYS — recent-only run over the last %v: boards without a date filter are skipped, nothing is closed and no board health is recorded", recentWindow)
	} else {
		log.Printf("ingest: full crawl")
	}
	bodies, err := sweeppolicy.BodyRefreshFor(os.Getenv("BODY_REFRESH_DAYS"), os.Getenv("BODY_REFRESH_SLICE"), time.Now())
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	bodies = bodies.WithProviderDefault(provider, time.Now()).WithDetailReread(provider, time.Now())
	if bodies.Enabled() {
		log.Printf("ingest: re-reading stored bodies older than %s, slice %d of %d",
			bodies.Cutoff.Format(time.DateOnly), bodies.Slot, bodies.Slices)
	}
	seen := seenPolicy{hydrationWindow: hydrationWindow, refetchAll: refetchAll, bodies: bodies}
	budget, err := runBudgetFor(os.Getenv("INGEST_RUN_TIMEOUT_SECONDS"), os.Getenv("INGEST_SWEEP_RESERVE_SECONDS"))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	boards, err := boardcatalog.LoadForProvider(ctx, boardcatalog.NewQueriesRepository(db.New(pool)), provider)
	if err != nil {
		log.Printf("config: load boards for %s: %v", provider, err)
		return 1
	}
	sourceCfg := sources.Config{Provider: provider, Sources: boards}

	// Computed against the FULL, unsharded board list: sharding can otherwise hide a
	// region-ambiguous board (see pipeline.AmbiguousBoardNames).
	crossShardAmbiguousBoards := pipeline.AmbiguousBoardNames(boards, registry)

	// Narrow to one board (optionally one region). A typo must not look like a legitimate
	// empty targeted run; an operator who got zero boards back needs to see why.
	if boardFilter != "" || regionFilter != "" {
		full := len(sourceCfg.Sources)
		sourceCfg = sourceCfg.FilterBoard(boardFilter, regionFilter)
		if len(sourceCfg.Sources) == 0 {
			log.Printf("config: --board=%q --region=%q matched none of %s's %d board(s)",
				boardFilter, regionFilter, provider, full)
			return 1
		}
		log.Printf("ingest: board filter — crawling %d of %d boards for %s (board=%q region=%q)",
			len(sourceCfg.Sources), full, provider, boardFilter, regionFilter)
	}

	// Narrow to this shard's slice, if requested.
	if shardSpec != "" {
		i, n, err := sources.ParseShard(shardSpec)
		if err != nil {
			log.Printf("config: %v", err)
			return 1
		}
		full := len(sourceCfg.Sources)
		sourceCfg = sourceCfg.Shard(i, n)
		log.Printf("ingest: shard %d/%d — crawling %d of %d boards for %s", i, n, len(sourceCfg.Sources), full, provider)
	}

	// Start the boards waited on longest, so a deadline-cut run rotates through the catalog
	// (see orderOldestRunFirst). Best-effort: without the read the catalog order stands.
	if lastRun, err := loadLastRuns(ctx, db.New(pool), provider); err != nil {
		log.Printf("ingest: read last crawl times for %s, keeping catalog order: %v", provider, err)
	} else {
		orderOldestRunFirst(sourceCfg.Sources, lastRun)
	}

	// crawled records the company slugs each provider wrote this run; the sweep scopes to them.
	crawled := newCrawledSet()
	// tally records which of the two writes each posting took (see writeTally).
	tally := newWriteTally()
	store := newDBStore(pool, enrich.Version, crawled, tally, seen)
	runner := pipeline.Runner{
		Registry:    registry,
		Store:       store,
		Outbound:    newNativeDiscovery(pool, registry, store),
		BoardHealth: newBoardHealth(pool),
		// Same object as Store: its pool already serves the alias registry.
		Aliases:      store,
		RecentWindow: recentWindow,
		WriteReserve: budget.writeReserve(),
	}

	crawlCtx, cancelCrawl := budget.crawlContext(ctx, started)
	if reconcileLinkedIn {
		defer cancelCrawl()
		report, err := reconcileLinkedInOwnership(crawlCtx, db.New(pool), registry["linkedin"], runner.Outbound, reconcileAfter)
		log.Printf("ingest: LinkedIn ownership reconciliation: %+v error=%v", report, err)
		if err != nil || report.More {
			return 1
		}
		return 0
	}
	runStats, crawlErr := runner.Run(crawlCtx, sourceCfg.Sources)
	cancelCrawl()

	// Everything after the crawl runs on its own context, detached from the crawl's, so the
	// crawl deadline does not cancel closes the run already earned (see runBudget).
	sweepCtx, cancelSweep := budget.sweepContext(ctx, started)
	defer cancelSweep()

	// A crawl stopped early (deadline or signal) still persisted what it read; the sweep below
	// narrows itself to the boards it finished.
	truncated := crawlErr != nil
	if truncated {
		log.Printf("ingest: crawl stopped before every board was reached (%v) — sweeping only the boards it finished", crawlErr)
	}

	total := runStats.Total()
	log.Printf("ingest done: provider=%s providers=%d ingested=%d failed=%d skipped=%d unreadable=%d interrupted=%d cooled=%d quiet=%d recent_unsupported=%d truncated=%t outbound_discoveries=%d",
		strconv.Quote(provider), len(runStats), total.Ingested, total.Failed, total.Skipped, total.Unreadable, total.Interrupted, total.Cooled, total.Quiet, total.RecentUnsupported, truncated, total.Discovered)

	// A provider at 0% never took the cheap write: a hashed field churns between crawls,
	// costing a full row rewrite and an index push every pass.
	if s := tally.summary(); s != "" {
		log.Printf("ingest writes: provider=%s %s", provider, s)
	}

	// The exit code says whether the RUN is broken, not whether a board is: see crawlReachedNothing.
	var failed int
	if crawlReachedNothing(total) {
		failed++
	}
	// A truncated run did not crawl what it was asked to: degraded however cleanly it stopped.
	if truncated {
		failed++
	}

	// Post-run sweep (job-lifecycle spec): per provider, close open jobs unseen for the whole
	// grace window, only for companies this run crawled (crawled.slugs). Per provider so one
	// run never closes another's jobs; only providers that ingested a job, so a total outage
	// cannot mass-close a catalogue; per company so a PARTIAL run closes only what it saw.
	//
	// Deliberate under-close: a company is swept only when the run wrote a job for it, so a
	// board that returned zero postings, or a removed company, is NOT retired here and its
	// open jobs leak until a later crawl. Board sources have no liveness backstop, and this
	// beats closing live jobs of boards a partial run never reached.
	//
	// A TRUNCATED run skips the company and source scopes and closes only through the board
	// scope. One employer can sit on several boards of a provider, and a deadline-stopped crawl
	// reaches the same leading boards every cycle, so a company-scoped close would retire the
	// unreached boards' postings run after run. The board scope closes only within a board read
	// to completion (pipeline.boardQualifies, which an interrupted board fails).
	queries := db.New(pool)
	failed += sweep(sweepCtx, queries, registry, runStats, crawled, crossShardAmbiguousBoards, truncated, recentWindow > 0)

	// One line naming failing/cooled boards, their failure count and next eligibility. After
	// the sweep: the closes are what the reserve is for.
	logUnhealthyBoards(sweepCtx, queries)
	logChronicBoards(sweepCtx, queries)

	return worker.ExitCode(failed, 0)
}

// crawlReachedNothing reports a run that tried and wrote nothing: boards or saves failed and
// no posting was saved or refreshed (host blocked, database unreachable, schema drift). A few
// failing boards among thousands are not that and must not turn a clean crawl into exit 1;
// those are board_health's to report.
func crawlReachedNothing(total pipeline.Stats) bool {
	return total.Ingested == 0 && total.Refreshed == 0 && (total.Failed > 0 || total.Skipped > 0)
}

// sweep runs the post-crawl stale-job closes for every provider in runStats and returns how many
// of them failed. ctx must be the sweep's own context (runBudget.sweepContext), never the crawl's.
// truncated narrows every provider to the board scope — see the comment at its call site.
func sweep(ctx context.Context, queries *db.Queries, registry map[string]sources.Source, runStats pipeline.RunStats, crawled *crawledSet, crossShardAmbiguousBoards map[string]bool, truncated, recent bool) int {
	// A recent-only listing omits older postings by construction, so it proves nothing about
	// what is gone: no scope at all.
	if recent {
		return 0
	}
	var failed int
	now := time.Now()
	// A slice-crawled source (e.g. whatjobs) declares a window wider than staleAfter: its crawl
	// reaches only a keyword's first pages, so deeper postings read as unseen and would be
	// closed and reopened. A full-board source with a tight cadence (e.g. gem) may declare a
	// narrower one, since its unseen reading is positive evidence.
	grace := sources.SweepGraceWindows(registry)
	// A self-closing source (e.g. jobtech) manages its own closes; it re-reports only changed
	// ads, so the unseen sweep would wrongly close every untouched open ad.
	sweepExempt := make(map[string]bool)
	for _, p := range sources.UnseenSweepExemptProviders(registry) {
		sweepExempt[p] = true
	}
	// A fullCatalog source (e.g. habr_career) lists its whole catalogue each run, so a clean run
	// may close its unseen jobs by source alone, retiring a vanished company that the company
	// scope leaks. Gated on a zero-Failed run below, since a truncated crawl must not
	// source-close what it never reached.
	fullCatalog := make(map[string]bool)
	for _, p := range sources.FullCatalogProviders(registry) {
		fullCatalog[p] = true
	}
	// A fullBoardListing provider's adapter is registered as proving it lists a board to
	// completion, so the sweep may close within its boards; see sweeppolicy.SweepableBoards for
	// the full gate.
	fullBoardListing := sources.FullBoardListingProviders(registry)
	// A sliceBoardListing provider (e.g. whatjobs) reads each board only to a page budget, so it
	// cannot prove a full listing — but its grace window absorbs that, so its finished boards
	// close within themselves on that window (see sweepableBoards).
	sliceBoardListing := sources.SliceBoardListingProviders(registry)
	// An emptyBoardListing provider's adapter answers an emptied board distinguishably from a
	// failed fetch, so a finished board that listed nothing joins its board scope too.
	emptyBoardListing := sources.EmptyBoardListingProviders(registry)
	for _, provider := range sweeppolicy.SweepableProviders(runStats) {
		if sweepExempt[provider] {
			continue
		}
		_, hasGrace := grace[provider]
		window := sweeppolicy.SweepWindowFor(grace, provider)
		cutoff := pgtype.Timestamptz{Time: now.Add(-window), Valid: true}
		// Board evidence admits this provider to dispatch without granting the broader
		// company/source scope: that still requires an ingested or refreshed posting.
		if !truncated && sweeppolicy.ShouldSweep(runStats[provider]) {
			bySource := sweeppolicy.SweepBySource(runStats[provider], fullCatalog[provider])
			companySlugs := sweeppolicy.SweepableCompanies(crawled.slugs(provider), runStats[provider])
			closed, skipped, err := sweepProvider(ctx, queries, provider, cutoff, companySlugs, runStats[provider].SkippedBoards, bySource)
			if err != nil {
				// Count and continue: one provider's sweep failure must not skip the rest,
				// but the run still exits non-zero.
				failed++
				log.Printf("close stale jobs (%s): %v", provider, err)
				continue
			}
			if skipped > 0 {
				failed++
				log.Printf("close stale jobs (%s): closed %d, skipped %d unclosable row(s) — see preceding lines for their ids", provider, closed, skipped)
			}
			log.Printf("closed %d stale %s jobs (unseen for %s)", closed, provider, window)
		}

		// Board-scoped close (job-lifecycle spec): the company scope leaks a company whose LAST
		// posting drops off a board this run still crawled, since its slug never re-enters
		// companySlugs. This closes per board the run proved it covered
		// (runStats[provider].QualifyingBoards); see sweeppolicy.SweepableBoards for the gate.
		// Uses the provider's own cutoff: the default for a complete listing, the grace window
		// for a slice.
		boards := sweeppolicy.SweepableBoards(runStats[provider], sweeppolicy.BoardScope{
			HasGrace:          hasGrace,
			FullCatalog:       fullCatalog[provider],
			FullBoardListing:  fullBoardListing[provider],
			SliceBoardListing: sliceBoardListing[provider],
			EmptyBoardListing: emptyBoardListing[provider],
			RegionScoped:      sources.ScopesBoardsByRegion(registry[provider]),
		}, crossShardAmbiguousBoards)
		if len(boards) > 0 {
			boardClosed, boardFailed := sweepBoards(ctx, queries, provider, cutoff, boards, registry[provider])
			log.Printf("closed %d stale %s jobs across %d finished board(s) (unseen for %s)", boardClosed, provider, len(boards), window)
			if boardFailed > 0 {
				failed++
			}
		}
	}
	return failed
}

// sweepBoards closes every qualifying board's unseen jobs in one statement per url scope, one
// scan of the provider's open rows rather than a round trip per board. Every provider but a
// region-scoped one has a single (empty) scope; a region-scoped one (jobstreet) gets one statement
// per region, narrowed to its url prefix (sources.BoardURLPrefix). sweeppolicy.SweepableBoards
// has already cleared the region of every board of an unscoped provider.
func sweepBoards(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, boards []pipeline.BoardRef, src sources.Source) (closed int64, failed int) {
	byScope := make(map[string][]string)
	var scopes []string
	for _, b := range boards {
		urlPattern := ""
		if b.Region != "" {
			prefix, ok := sources.BoardURLPrefix(src, b.Region)
			if !ok {
				// The crawl itself fails on a region its adapter does not know, so this board
				// cannot have qualified; refuse rather than close it unscoped.
				failed++
				log.Printf("close stale jobs (%s board %q): no url scope for region %q", provider, b.Board, b.Region)
				continue
			}
			urlPattern = externalid.LikePrefix(prefix)
		}
		if _, seen := byScope[urlPattern]; !seen {
			scopes = append(scopes, urlPattern)
		}
		byScope[urlPattern] = append(byScope[urlPattern], b.Board)
	}
	for _, urlPattern := range scopes {
		n, f := sweepBoardScope(ctx, queries, provider, cutoff, byScope[urlPattern], urlPattern)
		closed += n
		failed += f
	}
	return closed, failed
}

// sweepBoardScope closes the unseen jobs of boards sharing one url scope in one statement. If
// that fails it falls back to one statement per board (see sweepRowByRow), so an unwritable row
// blocks only its own board. failed counts the boards whose close failed.
func sweepBoardScope(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, boards []string, urlPattern string) (closed int64, failed int) {
	patterns := make([]string, len(boards))
	for i, board := range boards {
		patterns[i] = externalid.BoardPattern(board)
	}
	closed, err := queries.CloseUnseenJobsForBoards(ctx, db.CloseUnseenJobsForBoardsParams{
		Source:        provider,
		Cutoff:        cutoff,
		BoardPatterns: patterns,
		UrlPattern:    urlPattern,
	})
	if err == nil {
		return closed, 0
	}
	// An ended context would fail every per-board statement the same way; say so once.
	if ctx.Err() != nil {
		log.Printf("close stale jobs (%s, %d boards): %v", provider, len(boards), err)
		return 0, len(boards)
	}
	log.Printf("close stale jobs (%s, %d boards): batched close failed (%v), falling back to one board at a time", provider, len(boards), err)
	for i, board := range boards {
		n, err := queries.CloseUnseenJobsForBoard(ctx, db.CloseUnseenJobsForBoardParams{
			Source:       provider,
			Cutoff:       cutoff,
			BoardPattern: patterns[i],
			UrlPattern:   urlPattern,
		})
		if err != nil {
			failed++
			log.Printf("close stale jobs (%s board %q): %v", provider, board, err)
			continue
		}
		closed += n
	}
	return closed, failed
}

// sweepProvider closes one provider's unseen jobs: the bulk UPDATE (CloseUnseenJobs or
// CloseUnseenJobsBySource) is the fast path; on error it falls back to sweepRowByRow so one
// unwritable row doesn't block the provider. skipped is 0 unless the fallback ran.
// skippedBoards (pipeline.Stats.SkippedBoards) are left out of either scope: the run did not
// look at them, so their rows' age says nothing about whether they are still listed.
func sweepProvider(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, companySlugs, skippedBoards []string, bySource bool) (closed int64, skipped int, err error) {
	if bySource {
		closed, err = queries.CloseUnseenJobsBySource(ctx, db.CloseUnseenJobsBySourceParams{
			Source:               provider,
			Cutoff:               cutoff,
			SkippedBoardPatterns: skippedBoards,
		})
	} else {
		closed, err = queries.CloseUnseenJobs(ctx, db.CloseUnseenJobsParams{
			Source:               provider,
			Cutoff:               cutoff,
			CompanySlugs:         companySlugs,
			SkippedBoardPatterns: skippedBoards,
		})
	}
	if err == nil {
		return closed, 0, nil
	}
	log.Printf("close stale jobs (%s): bulk close failed (%v), falling back to row-by-row", provider, err)
	closed, skipped, err = sweepRowByRow(ctx, queries, provider, cutoff, companySlugs, skippedBoards, bySource)
	if err != nil {
		return 0, 0, fmt.Errorf("row-by-row fallback failed: %w", err)
	}
	return closed, skipped, nil
}

// sweepRowByRow is the bulk sweep's fallback when the single-statement UPDATE fails: it
// fetches the same candidate set and closes each row in its own statement, so one row Postgres
// can't write (e.g. a corrupted jobs_pkey value) is skipped by itself instead of aborting the
// provider. Slower than the bulk path; it only runs after the fast path failed.
func sweepRowByRow(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, companySlugs, skippedBoards []string, bySource bool) (closed int64, skipped int, err error) {
	var ids []int64
	if bySource {
		ids, err = queries.UnseenJobIDsBySource(ctx, db.UnseenJobIDsBySourceParams{Source: provider, Cutoff: cutoff, SkippedBoardPatterns: skippedBoards})
	} else {
		ids, err = queries.UnseenJobIDs(ctx, db.UnseenJobIDsParams{Source: provider, Cutoff: cutoff, CompanySlugs: companySlugs, SkippedBoardPatterns: skippedBoards})
	}
	if err != nil {
		return 0, 0, fmt.Errorf("list candidates: %w", err)
	}
	for _, id := range ids {
		n, err := queries.CloseUnseenJobByID(ctx, db.CloseUnseenJobByIDParams{ID: id, Cutoff: cutoff})
		if err != nil {
			skipped++
			log.Printf("close stale jobs (%s): skipping id=%d, still unclosable: %v", provider, id, err)
			continue
		}
		closed += n
	}
	return closed, skipped, nil
}
