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
// Each crawl's new or content-changed jobs are queued (search_outbox, atomically
// with their write) for the live facet search index; cmd/search-drain builds and
// pushes the documents on its own schedule. Routing every write through that queue —
// instead of pushing to Meilisearch directly from inside this worker, one of ~169
// independent per-board processes — collapses many small, expensive index merges into
// few, cheap ones. The batch reindex stays the index's source of truth.
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
	// The unit's timeout is measured from when it started, which is as close to here as this
	// process can observe — so the crawl deadline is measured from here too.
	started := time.Now()

	// The provider to crawl. Accept it as the first positional argument (cron passes the
	// provider name) or via INGEST_PROVIDER. An optional --shard=i/n (or the SHARD env)
	// crawls only a round-robin slice of that provider's boards, so a provider with too
	// many boards to finish in one timeout (workday) is spread across several staggered
	// runs. --board=<id> (optionally narrowed further by --region=<code>) is the operator's
	// hand-run equivalent: resync one company/keyword — e.g. one jobleads keyword, one
	// workday tenant — without waiting for or disturbing the rest of the provider's crawl.
	var provider, shardSpec, boardFilter, regionFilter string
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--shard="):
			shardSpec = strings.TrimPrefix(a, "--shard=")
		case a == "--shard":
			// The value must be attached (--shard=i/n); a space-separated form would
			// otherwise swallow the next arg (the provider name) as the selector's value.
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

	registry := sources.All(sources.NewClient())
	// Route the IP-blocklisted providers through the egress proxy when one is configured
	// (SOURCES_PROXY_URL). No-op when unset; a set-but-invalid value fails the run here,
	// before the DB is touched.
	if err := sources.ApplyProxyEgress(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	// LAST, and the order is load-bearing: the Scrapling tier deliberately takes its providers
	// off the proxy and fingerprint transports wired above. See ApplyScraplingEgress.
	// A no-op without SCRAPLING_FETCH_URL, which is also the rollback.
	if err := sources.ApplyScraplingEgress(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// Resolved before the DB is touched, like every other config read here: a bad value should
	// stop the run, not produce a quiet ordinary crawl where a repair was intended.
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
	bodies = bodies.WithDetailReread(provider, time.Now())
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

	// Computed against the FULL, unsharded board list — see pipeline.AmbiguousBoardNames for
	// why sharding (below) can otherwise hide a region-ambiguous board from the board-scoped
	// close's per-run safety check.
	crossShardAmbiguousBoards := pipeline.AmbiguousBoardNames(boards, registry)

	// Narrow to one board (optionally one region), if requested. A typo here must not look
	// like a legitimate empty targeted run (see the sweep's own note on those below) — an
	// operator who asked for a specific board and got zero boards back needs to see why.
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

	// Start the boards waited on longest, so a run cut short by its deadline rotates through
	// the catalog instead of starving the same tail every time (see orderOldestRunFirst).
	// Best-effort: without the read the catalog order stands, which is only less fair.
	if lastRun, err := loadLastRuns(ctx, db.New(pool), provider); err != nil {
		log.Printf("ingest: read last crawl times for %s, keeping catalog order: %v", provider, err)
	} else {
		orderOldestRunFirst(sourceCfg.Sources, lastRun)
	}

	// crawled records the company slugs each provider actually wrote this run, so the
	// post-run sweep scopes its closes to them (see the sweep below and crawledSet).
	crawled := newCrawledSet()
	// tally records which of the two writes each persisted posting took, so the run says how far
	// the cheap path actually reached rather than leaving it to be assumed (see writeTally).
	tally := newWriteTally()
	store := newDBStore(pool, enrich.Version, crawled, tally, seen)
	runner := pipeline.Runner{
		Registry:    registry,
		Store:       store,
		BoardHealth: newBoardHealth(pool),
		// The coverage gate reads the same pool: last_seen_at decides the answer, and no
		// index can hold it (see coverage.go). It was Meili-backed and gated on MeiliKey;
		// every production ingest unit already carried that key, so the gate is on for the
		// same runs as before — only a local run without search configured gains it.
		Coverage: newCoverage(pool),
		// Same object as Store: the alias registry is a read the store's pool already
		// serves, and the pipeline asks for it once per board run.
		Aliases:      store,
		RecentWindow: recentWindow,
	}

	crawlCtx, cancelCrawl := budget.crawlContext(ctx, started)
	runStats, crawlErr := runner.Run(crawlCtx, sourceCfg.Sources)
	cancelCrawl()

	// Everything after the crawl runs on its own context, detached from the crawl's: the crawl
	// deadline exists precisely so this part still runs, and a context that died with the crawl
	// would cancel every close the run had already earned (see runBudget).
	sweepCtx, cancelSweep := budget.sweepContext(ctx, started)
	defer cancelSweep()

	// A crawl that stopped early — its deadline, or a signal — still persisted everything it
	// read and still proved the boards it finished; the sweep below narrows itself to those.
	truncated := crawlErr != nil
	if truncated {
		log.Printf("ingest: crawl stopped before every board was reached (%v) — sweeping only the boards it finished", crawlErr)
	}

	total := runStats.Total()
	log.Printf("ingest done: provider=%s providers=%d ingested=%d failed=%d skipped=%d rejected=%d unreadable=%d interrupted=%d cooled=%d quiet=%d recent_unsupported=%d truncated=%t",
		provider, len(runStats), total.Ingested, total.Failed, total.Skipped, total.Rejected, total.Unreadable, total.Interrupted, total.Cooled, total.Quiet, total.RecentUnsupported, truncated)

	// A provider at 0% took the cheap write for nothing all run — a hashed field is churning
	// between crawls, which costs a full row rewrite AND a pointless index push every pass.
	if s := tally.summary(); s != "" {
		log.Printf("ingest writes: provider=%s %s", provider, s)
	}

	// The exit code says whether the RUN is broken, not whether a board is: see crawlReachedNothing.
	var failed int
	if crawlReachedNothing(total) {
		failed++
	}
	// A truncated run did not crawl what it was asked to, which is a degraded run however
	// cleanly it stopped; the exit code says so, as it did when the unit's timeout killed it.
	if truncated {
		failed++
	}

	// Post-run sweep (job-lifecycle spec): per provider, close that provider's open jobs
	// unseen for the whole grace window — but only for the companies this run actually
	// crawled (crawled.slugs). Scoped per provider so one provider's run never closes
	// another's jobs; guarded per provider (only those that ingested at least one job) so
	// a total crawl outage cannot mass-close a catalogue; and scoped per company so a
	// PARTIAL run (a subset of a provider's boards — a targeted run, or a full crawl of a
	// huge provider that timed out mid-way) closes only what it saw, never the boards it
	// never reached.
	//
	// Trade-off (deliberate under-close): a company is swept only when the run wrote a job
	// for it, so a board that fetched but returned zero postings, or a company removed from
	// the board file, is NOT retired here — its open jobs leak until a later crawl reopens
	// or closes them. Board sources have no liveness backstop (the liveness probe skips
	// registered providers), so this is accepted to avoid the far worse over-close: closing
	// live jobs of boards a partial/timed-out run never reached.
	//
	// A TRUNCATED run skips the company scope (and the source scope) and closes only through
	// the board scope. A company is not a board: one employer can sit on several boards of one
	// provider (a workday tenant's several career sites), and a crawl stopped by its deadline
	// reaches the same leading boards every cycle — so a company written by a board it finished
	// would close the stale postings of that company's boards it never reaches, run after run,
	// with nothing re-crawling them to reopen them. The board scope has no such gap: it closes
	// only within a board the run read to completion (pipeline.boardQualifies, which an
	// interrupted board fails). Before the crawl deadline existed a timed-out run swept nothing
	// at all, so this withholds nothing a run used to do.
	queries := db.New(pool)
	failed += sweep(sweepCtx, queries, registry, runStats, crawled, crossShardAmbiguousBoards, truncated, recentWindow > 0)

	// Surface which boards are failing / cooled without grepping logs — one line naming them,
	// their failure count, and when they next become eligible. After the sweep rather than
	// before it: these are reports, and the closes are what the reserve is for.
	logUnhealthyBoards(sweepCtx, queries)
	logChronicBoards(sweepCtx, queries)

	return worker.ExitCode(failed, 0)
}

// crawlReachedNothing reports a run that tried and wrote nothing: boards failed or saves failed,
// and not one posting was saved or refreshed — the host blocked, the database unreachable, a
// schema drift. One failing board among thousands is not that. A provider of thousands of boards
// always has a few that 404 or 401 this hour, and counting each one made a clean crawl exit 1:
// on 2026-11-05, 17 static ingest units were red for exactly that (ashby failed=5 beside 26k
// postings written), so a red unit meant nothing. A failing board is board_health's to report —
// the run's unhealthy/chronic summary and close-chronic-boards — not the exit code's.
func crawlReachedNothing(total pipeline.Stats) bool {
	return total.Ingested == 0 && total.Refreshed == 0 && (total.Failed > 0 || total.Skipped > 0)
}

// sweep runs the post-crawl stale-job closes for every provider in runStats and returns how many
// of them failed. ctx must be the sweep's own context (runBudget.sweepContext), never the crawl's.
// truncated narrows every provider to the board scope — see the comment at its call site.
func sweep(ctx context.Context, queries *db.Queries, registry map[string]sources.Source, runStats pipeline.RunStats, crawled *crawledSet, crossShardAmbiguousBoards map[string]bool, truncated, recent bool) int {
	// A recent-only listing omits every older posting by construction, so it proves nothing about
	// what is gone. Stronger than the truncated run's board scope: no scope at all.
	if recent {
		return 0
	}
	var failed int
	now := time.Now()
	// A slice-crawled source (e.g. whatjobs) declares a window wider than staleAfter: its crawl
	// reaches only a keyword's first pages, so a posting that drifted deeper reads as unseen and
	// the default window would close it and reopen it on the next run. A full-board source with a
	// reliably tight crawl cadence (e.g. gem) may instead declare one narrower, since its unseen
	// reading is already positive evidence rather than a guess about how deep the crawl got.
	grace := sources.SweepGraceWindows(registry)
	// A self-closing source (e.g. jobtech) manages its own closes from its stream, so the
	// unseen sweep must skip it: it re-reports only changed ads, and the cutoff would wrongly
	// close every still-open ad it did not touch this run.
	selfClosing := make(map[string]bool)
	for _, p := range sources.SelfClosingProviders(registry) {
		selfClosing[p] = true
	}
	// A fullCatalog source (e.g. habr_career) lists its whole catalogue each run, so a clean run
	// may close its unseen jobs by source alone — retiring a company that vanished from the feed,
	// which the company-scoped close leaks. Gated on a zero-Failed run below (a truncated crawl,
	// which such adapters surface as an error, must not source-close what it never reached).
	fullCatalog := make(map[string]bool)
	for _, p := range sources.FullCatalogProviders(registry) {
		fullCatalog[p] = true
	}
	// A fullBoardListing provider's adapter is registered as structurally proving it lists a
	// board to completion (freehire#2328), so the sweep may close within its boards — see
	// sweeppolicy.SweepableBoards for the full gate, including why a sweepGrace or fullCatalog provider is
	// excluded even when registered.
	fullBoardListing := sources.FullBoardListingProviders(registry)
	// A sliceBoardListing provider (e.g. whatjobs) reads each board only to a page budget, so it
	// cannot prove a full listing — but its grace window absorbs that, so its finished boards
	// close within themselves on that window (see sweepableBoards).
	sliceBoardListing := sources.SliceBoardListingProviders(registry)
	// An emptyBoardListing provider's adapter answers an emptied board distinguishably from a
	// failed fetch, so a finished board that listed nothing joins its board scope too.
	emptyBoardListing := sources.EmptyBoardListingProviders(registry)
	for _, provider := range sweeppolicy.SweepableProviders(runStats) {
		if selfClosing[provider] {
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

		// Board-scoped close (job-lifecycle spec, freehire#2328): the company scope above
		// leaks a company whose LAST posting drops off a board this run still crawled — that
		// company's slug never re-enters companySlugs, so its row never closes under either
		// scope so far. This closes it directly, per board the run structurally proved it
		// covered (runStats[provider].QualifyingBoards) on a provider whose adapter is
		// registered as listing a board to completion, or as a bounded slice whose grace window
		// stands in for that proof — see sweeppolicy.SweepableBoards for the full gate. Reuses this
		// provider's own cutoff: the default for a complete listing, the grace window for a slice.
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

// defaultSweepReserve is how much of the run's timeout is held back from the crawl for what
// follows it. Measured on the crawl host over 2026-10-21..23, from each run's "ingest done" line
// to its last close: the slowest sweeps were workday shards at 570-582s (the company-scope UPDATE
// alone 377-471s of it) and vagas at 1054s; nearly every other provider finished inside two
// minutes. Ten minutes covers workday's; a provider whose sweep runs longer (vagas) sets
// INGEST_SWEEP_RESERVE_SECONDS rather than taxing every crawl for it.
const defaultSweepReserve = 10 * time.Minute

// runBudget splits a run's timeout between the crawl and the sweep that follows it.
//
// The unit's timeout is a hard kill: systemd sends SIGTERM, the signal context cancels, and
// whatever was in flight dies with it. When the crawl was what ran long, the sweep was what died
// — from 2026-10-18, when the crawl moved a WireGuard hop away from the database, 0 of 33
// greenhouse runs, 0 of 31 smartrecruiters and 0 of 29 ukg runs reached it, and ~307k dead
// workday postings stayed open behind boards board_health called healthy. So the crawl gets a
// deadline of its own, reserve short of the timeout, and stops there; the sweep then has the
// reserve to itself.
//
// timeout is zero when INGEST_RUN_TIMEOUT_SECONDS is unset — a process cannot read its own
// unit's TimeoutStartSec — and the crawl then has no deadline of its own, exactly as before. The
// scheduler passes the value with every launch (ingestsched.SystemdLauncher); a static unit must
// set it in its environment, net of any time a wrapper may spend waiting for a slot.
type runBudget struct {
	timeout time.Duration
	reserve time.Duration
}

// runBudgetFor resolves INGEST_RUN_TIMEOUT_SECONDS and INGEST_SWEEP_RESERVE_SECONDS. A set value
// must be a positive number of seconds, and the reserve must leave the crawl something: a reserve
// at or past the timeout would end every crawl before its first board, a misconfiguration that
// reads exactly like a quiet fleet.
func runBudgetFor(timeoutEnv, reserveEnv string) (runBudget, error) {
	b := runBudget{reserve: defaultSweepReserve}
	if reserveEnv != "" {
		secs, err := strconv.Atoi(reserveEnv)
		if err != nil || secs <= 0 {
			return runBudget{}, fmt.Errorf("INGEST_SWEEP_RESERVE_SECONDS must be a positive number of seconds, got %q", reserveEnv)
		}
		b.reserve = time.Duration(secs) * time.Second
	}
	if timeoutEnv == "" {
		return b, nil
	}
	secs, err := strconv.Atoi(timeoutEnv)
	if err != nil || secs <= 0 {
		return runBudget{}, fmt.Errorf("INGEST_RUN_TIMEOUT_SECONDS must be a positive number of seconds, got %q", timeoutEnv)
	}
	b.timeout = time.Duration(secs) * time.Second
	if b.reserve >= b.timeout {
		return runBudget{}, fmt.Errorf("INGEST_SWEEP_RESERVE_SECONDS (%s) must be shorter than INGEST_RUN_TIMEOUT_SECONDS (%s), or no crawl can start",
			b.reserve, b.timeout)
	}
	return b, nil
}

// crawlContext bounds the crawl at reserve short of the run's timeout. Without a known timeout
// it is parent, cancellable.
func (b runBudget) crawlContext(parent context.Context, started time.Time) (context.Context, context.CancelFunc) {
	if b.timeout == 0 {
		return context.WithCancel(parent)
	}
	return context.WithDeadline(parent, started.Add(b.timeout-b.reserve))
}

// sweepContext is what the sweep runs on: detached from parent's cancellation, so neither the
// crawl's deadline nor a signal that stopped the crawl cancels a close, and bounded by the run's
// timeout — the moment the unit kills the process anyway. Without a known timeout it is bounded
// by the reserve from now, which is what the reserve exists to give it.
//
// Detaching from the SIGNAL too is deliberate. A close is one atomic statement, so the worst a
// later kill can do is roll the one in flight back; honouring the signal instead would discard
// the closes a run already earned in exactly the case — a timed-out run on a unit that does not
// yet pass its timeout — this exists to fix.
func (b runBudget) sweepContext(parent context.Context, started time.Time) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(parent)
	if b.timeout == 0 {
		return context.WithTimeout(detached, b.reserve)
	}
	return context.WithDeadline(detached, started.Add(b.timeout))
}

// sweepBoards closes every qualifying board's unseen jobs in one statement per url scope — one
// scan of the provider's open rows rather than one round trip per board (see
// CloseUnseenJobsForBoards for the measurement). Every provider but a region-scoped one has a
// single scope, ”; a region-scoped one (jobstreet) gets one statement per region, narrowed to
// the url prefix its postings live on (sources.BoardURLPrefix). sweeppolicy.SweepableBoards has already
// cleared the region of every board of an unscoped provider.
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
// that statement fails it falls back to one statement per board, so a row Postgres cannot write
// (the 2026-08-11 incident sweepRowByRow exists for) blocks only its own board rather than every
// board of the run. failed counts the boards whose close failed.
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

// hydrationRetryWindowFor resolves how long a stored row with no description keeps being
// re-offered to a hydrating adapter for another detail fetch (see pipeline.SeenLookup). An
// empty value means the default, pipeline.HydrationRetryWindow.
//
// HYDRATION_RETRY_DAYS widens it for a deliberate repair run: the default window is measured
// from created_at, so a backlog that accumulated before the retry existed has already aged past
// it and no ordinary crawl will ever re-fetch those bodies. `HYDRATION_RETRY_DAYS=365 ingest
// sources/hh.yml` re-offers every body-less row of that provider instead. Expect the run to be
// slower by one detail request per such row (hh's detail pages are ~1 MB), and expect to repeat
// it: a run that hits its systemd timeout still persists what it hydrated, and the rows it
// fixed leave the set, so successive runs shrink the backlog rather than redoing it.
//
// An unparseable or non-positive value is an error rather than a fallback to the default. This
// is set by hand for a one-off, where silently ingesting with the default would look exactly
// like a repair run that found nothing to repair.
func hydrationRetryWindowFor(env string) (time.Duration, error) {
	if env == "" {
		return pipeline.HydrationRetryWindow, nil
	}
	days, err := strconv.Atoi(env)
	if err != nil || days <= 0 {
		return 0, fmt.Errorf("HYDRATION_RETRY_DAYS must be a positive number of days, got %q", env)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// refetchAllFor resolves INGEST_REFETCH_ALL, the repair switch that empties the seen-set so a
// crawl re-WRITES the provider's stored rows instead of only refreshing their liveness (see
// dbStore.ExistingExternalIDs). It is what carries an adapter fix to the postings ingested before
// it: the ordinary crawl never rewrites them, and re-deriving from the database cannot recover a
// field the adapter read wrong, because what the database stored is the wrong reading.
//
// Set it by hand for one run and expect the crawl to cost one detail request per stored posting.
// Nothing is lost if a detail request fails — UpsertJob keeps the stored description when the
// incoming one is empty — so a run that hits its systemd timeout can simply be repeated.
//
// Anything other than the two accepted spellings is an error rather than a quiet false: this is a
// hand-set one-off, where an ordinary crawl would look exactly like a repair that found nothing.
func refetchAllFor(env string) (bool, error) {
	switch env {
	case "":
		return false, nil
	case "1", "true":
		return true, nil
	default:
		return false, fmt.Errorf("INGEST_REFETCH_ALL must be 1 or true when set, got %q", env)
	}
}

// maxRecentDays caps INGEST_RECENT_DAYS. A window this wide is already a near-full read, and a
// typo like 365 would pass for a recent run while asking a source for a year of postings.
const maxRecentDays = 30

// recentWindowFor reads INGEST_RECENT_DAYS: unset means a full crawl (0), otherwise a positive
// whole number of days up to maxRecentDays. A bad value fails the run rather than falling back,
// because a typo here would otherwise start an expensive full crawl where a cheap one was meant.
func recentWindowFor(env string) (time.Duration, error) {
	if env == "" {
		return 0, nil
	}
	days, err := strconv.Atoi(env)
	if err != nil || days < 1 || days > maxRecentDays {
		return 0, fmt.Errorf("INGEST_RECENT_DAYS must be a whole number of days from 1 to %d when set, got %q", maxRecentDays, env)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// sweepProvider closes one provider's unseen jobs: the bulk UPDATE (CloseUnseenJobs or
// CloseUnseenJobsBySource) is the fast path, and on error it falls back to sweepRowByRow
// so a single row Postgres can't write doesn't block the rest of the provider — see
// sweepRowByRow for why that happens. skipped is always 0 unless the fallback ran.
// skippedBoards (pipeline.Stats.SkippedBoards) are left out of either scope: the run did not look
// at them, so their rows' age says nothing about whether they are still listed.
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
// fetches the same candidate set and closes each row in its own statement, so one row
// Postgres can't write (e.g. a heap/index-corrupted jobs_pkey value — see the 2026-08-11
// incident, where one such row blocked greenhouse's sweep on every run) is skipped by
// itself instead of aborting every other closeable row in the provider. Slower than the
// bulk path by design: it only runs once the fast path has already failed.
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
		n, err := queries.CloseUnseenJobByID(ctx, id)
		if err != nil {
			skipped++
			log.Printf("close stale jobs (%s): skipping id=%d, still unclosable: %v", provider, id, err)
			continue
		}
		closed += n
	}
	return closed, skipped, nil
}
