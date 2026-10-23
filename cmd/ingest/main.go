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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// staleAfter is the DEFAULT grace window before an unseen job is closed. An adapter declares
// its own window instead — wider for one that crawls only a slice of its catalogue, narrower
// for one whose full-board crawl makes an unseen reading positive evidence — see sweepWindowFor.
// Shared with cmd/liveness's staleCutoff via sources.DefaultSweepGrace, since liveness's
// probeDespiteRegistered backstop only picks up what this sweep has already had a chance
// to close and must not drift out of sync with it.
const staleAfter = sources.DefaultSweepGrace

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
	bodies, err := bodyRefreshFor(os.Getenv("BODY_REFRESH_DAYS"), os.Getenv("BODY_REFRESH_SLICE"), time.Now())
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if bodies.enabled() {
		log.Printf("ingest: re-reading stored bodies older than %s, slice %d of %d",
			bodies.cutoff.Format(time.DateOnly), bodies.slot, bodies.slices)
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
	crossShardAmbiguousBoards := pipeline.AmbiguousBoardNames(boards)

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
		Aliases: store,
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
	log.Printf("ingest done: provider=%s providers=%d ingested=%d failed=%d skipped=%d rejected=%d unreadable=%d interrupted=%d truncated=%t",
		provider, len(runStats), total.Ingested, total.Failed, total.Skipped, total.Rejected, total.Unreadable, total.Interrupted, truncated)

	// A provider at 0% took the cheap write for nothing all run — a hashed field is churning
	// between crawls, which costs a full row rewrite AND a pointless index push every pass.
	if s := tally.summary(); s != "" {
		log.Printf("ingest writes: provider=%s %s", provider, s)
	}

	// A failed board is counted in total.Failed; surface it (and any sweep failure
	// below) through the exit code so cron alerts on a degraded run.
	failed := total.Failed
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
	failed += sweep(sweepCtx, queries, registry, runStats, crawled, crossShardAmbiguousBoards, truncated)

	// Surface which boards are failing / cooled without grepping logs — one line naming them,
	// their failure count, and when they next become eligible. After the sweep rather than
	// before it: these are reports, and the closes are what the reserve is for.
	logUnhealthyBoards(sweepCtx, queries)
	logChronicBoards(sweepCtx, queries)

	return worker.ExitCode(failed, 0)
}

// sweep runs the post-crawl stale-job closes for every provider in runStats and returns how many
// of them failed. ctx must be the sweep's own context (runBudget.sweepContext), never the crawl's.
// truncated narrows every provider to the board scope — see the comment at its call site.
func sweep(ctx context.Context, queries *db.Queries, registry map[string]sources.Source, runStats pipeline.RunStats, crawled *crawledSet, crossShardAmbiguousBoards map[string]bool, truncated bool) int {
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
	// sweepableBoards for the full gate, including why a sweepGrace or fullCatalog provider is
	// excluded even when registered.
	fullBoardListing := sources.FullBoardListingProviders(registry)
	for _, provider := range sweepableProviders(runStats) {
		if selfClosing[provider] {
			continue
		}
		_, hasGrace := grace[provider]
		window := sweepWindowFor(grace, provider)
		cutoff := pgtype.Timestamptz{Time: now.Add(-window), Valid: true}
		if !truncated {
			bySource := sweepBySource(runStats[provider], fullCatalog[provider])
			companySlugs := sweepableCompanies(crawled.slugs(provider), runStats[provider])
			closed, skipped, err := sweepProvider(ctx, queries, provider, cutoff, companySlugs, bySource)
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
		// registered as listing a board to completion — see sweepableBoards for the full gate.
		// Reuses this provider's own cutoff: a board-scope candidate is by construction never
		// a sweepGrace provider, so the window is always the default here.
		boards := sweepableBoards(runStats[provider], hasGrace, fullCatalog[provider], fullBoardListing[provider], crossShardAmbiguousBoards)
		if len(boards) > 0 {
			boardClosed, boardFailed := sweepBoards(ctx, queries, provider, cutoff, boards)
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

// sweepBoards closes every qualifying board's unseen jobs in one statement — one scan of the
// provider's open rows rather than one round trip per board (see CloseUnseenJobsForBoards for
// the measurement). If that statement fails it falls back to one statement per board, so a row
// Postgres cannot write (the 2026-08-11 incident sweepRowByRow exists for) blocks only its own
// board rather than every board of the run. failed counts the boards whose close failed.
func sweepBoards(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, boards []string) (closed int64, failed int) {
	patterns := make([]string, len(boards))
	for i, board := range boards {
		patterns[i] = externalid.BoardPattern(board)
	}
	closed, err := queries.CloseUnseenJobsForBoards(ctx, db.CloseUnseenJobsForBoardsParams{
		Source:        provider,
		Cutoff:        cutoff,
		BoardPatterns: patterns,
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

// sweepableBoards returns, sorted and de-duplicated, the boards of one provider the
// board-scoped close may retire this run: those the run structurally proved it covered
// (stats.QualifyingBoards, see pipeline.boardQualifies), gated on three pre-resolved,
// per-provider facts the caller has already looked up — the same convention sweepBySource
// uses, rather than this function re-deriving them from the raw registry maps itself.
//
// fullBoardListing must be true: a provider whose adapter is not registered as listing a
// board to completion never contributes to the board scope, however its crawl went — see
// sources.fullBoardListing for the bar an adapter must clear to earn it. hasGrace and
// fullCatalog exclude a provider even when registered, for the same reasons sweepBySource
// excludes them from the source-scoped close: a sweepGrace provider's crawl deliberately
// reaches only a slice of the catalogue, so a board-scoped close on the default window would
// close postings that merely drifted past the crawl's depth; a fullCatalog provider already
// closes by source alone on a clean run, strictly broader than board scope (and today's
// fullCatalog adapters are boardless besides, so this exclusion is belt-and-braces). Callers
// gate on shouldSweep first, same as sweepBySource.
//
// crossShardAmbiguous excludes a board name this run's own Stats saw as unambiguous but that
// is region-ambiguous across the FULL, unsharded catalog (pipeline.AmbiguousBoardNames,
// computed by the caller before sharding): sources.Config.Shard groups boards by company
// slug, not by board name, so a board's two region-variant rows can land in separate shard
// processes — each running its own Runner.Run and seeing only one region, concluding on its
// own that the board is unambiguous. Refusing here is what catches what a single Run() call
// structurally cannot.
//
// De-duplication guards against a board legitimately appearing twice in one run (a repeated
// board-file entry, or one board id recurring across independent regional slices) double-
// counting the close and its log line.
func sweepableBoards(stats pipeline.Stats, hasGrace, fullCatalog, fullBoardListing bool, crossShardAmbiguous map[string]bool) []string {
	if !fullBoardListing || hasGrace || fullCatalog {
		return nil
	}
	boards := slices.Clone(stats.QualifyingBoards)
	sort.Strings(boards)
	boards = slices.Compact(boards)
	boards = slices.DeleteFunc(boards, func(board string) bool { return crossShardAmbiguous[board] })
	if len(boards) == 0 {
		return nil
	}
	return boards
}

// sweepableCompanies narrows the crawled-company scope to the companies this run may actually
// close within: those it wrote a job for (crawled), minus the ones a board withheld because its
// crawl did not read what it listed (pipeline.Stats.UnprovenCompanies).
//
// The subtraction is what keeps the two halves of the fix honest. A board whose detail requests
// died still WRITES everything it did manage to read — those rows' last_seen_at moves, their
// content is refreshed, their company enters the crawled set exactly as before — and only the
// permission to retire that company's unseen rows is withheld. Recording and closing are
// separate decisions, and only the second one lacks the evidence.
//
// It withholds by COMPANY rather than by board because the crawled scope is the only handle the
// provider-wide CloseUnseenJobs has; a company under two boards, one of which read cleanly, is
// still withheld, which is the conservative reading and the correct one — the other board's
// unread postings belong to that same company.
//
// Returns nil rather than an empty slice when nothing is left, which the close reads as an
// empty text[] and matches no row (`company_slug = ANY('{}')`), i.e. it closes nothing.
func sweepableCompanies(crawled []string, stats pipeline.Stats) []string {
	if len(stats.UnprovenCompanies) == 0 {
		return crawled
	}
	unproven := make(map[string]bool, len(stats.UnprovenCompanies))
	for _, slug := range stats.UnprovenCompanies {
		unproven[slug] = true
	}
	kept := slices.DeleteFunc(slices.Clone(crawled), func(slug string) bool { return unproven[slug] })
	if len(kept) == 0 {
		return nil
	}
	return kept
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

// sweepBySource reports whether a provider's run may close its unseen jobs by source alone,
// dropping the crawled-company scope. Only a fullCatalog source qualifies (it lists the whole
// catalogue each run, so an unseen job is truly gone), and only when the run had zero board
// failures: a fullCatalog adapter errors a truncated crawl, so Failed>0 means the listing was
// incomplete and a source-scoped close would mass-close the postings it never reached. Such a run
// falls back to the safe company-scoped CloseUnseenJobs. Callers gate on shouldSweep first.
//
// A posting the crawl could not READ disqualifies it too, and here with no tolerance at all
// (unlike the board scope's maxUnreadablePercent): this is the broadest close in the system, it
// drops the company scope that sweepableCompanies uses to withhold anything, and no fullCatalog
// adapter emits the marker today — so the strict reading costs nothing now and is the one that
// stays right if one ever does.
func sweepBySource(stats pipeline.Stats, fullCatalog bool) bool {
	return fullCatalog && stats.Failed == 0 && stats.Unreadable == 0
}

// sweepWindowFor reports how long a provider's unseen jobs are spared before the sweep closes
// them: the window its adapter declared (sources.SweepGraceWindows), wider or narrower than
// staleAfter, else staleAfter itself. The override is per-provider so one feed's drift tolerance
// (or one full-board crawl's tighter cadence) never changes the sweep for every other provider.
func sweepWindowFor(grace map[string]time.Duration, provider string) time.Duration {
	if w, ok := grace[provider]; ok {
		return w
	}
	return staleAfter
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

// bodyRefresh is the resolved "re-read a stale body" policy: how old a stored body may be
// before a hydrating crawl fetches it again, and which slice of the catalogue this run takes.
// See dbStore.ExistingExternalIDs and migration 0144.
type bodyRefresh struct {
	// cutoff is the age boundary: a row hydrated before it is stale. Meaningless while
	// disabled, since slot then matches nothing.
	cutoff time.Time
	// slices is how many runs a full sweep of the catalogue takes; always positive, because
	// it is a modulus.
	slices int64
	// slot is the slice this run takes, or bodyRefreshDisabledSlot.
	slot int64
}

// bodyRefreshDisabledSlot is a slot no row can hash to: the predicate computes
// abs(hashtext(external_id)) % slices, which is never negative. Disabling the feature is
// therefore a parameter value rather than a second query, so there is one predicate to reason
// about and no branch that could disagree with the other.
const bodyRefreshDisabledSlot = -1

// bodyRefreshDefaultSlices is how many runs a full sweep takes when BODY_REFRESH_SLICE is unset.
const bodyRefreshDefaultSlices = 30

func (b bodyRefresh) enabled() bool { return b.slot != bodyRefreshDisabledSlot }

// bodyRefreshFor resolves BODY_REFRESH_DAYS and BODY_REFRESH_SLICE.
//
// It exists because a hydrating adapter fetches a posting's detail only while the catalogue
// does not already hold it: once stored, a posting's body is never read again, so an employer
// editing their own posting — adding "this position is 100% on-site", moving the location,
// stating a salary — reaches us never (freehire#2555). BODY_REFRESH_DAYS is how old a stored
// body may get before the next crawl re-reads it.
//
// Unset means disabled, and that is the shipped default: turning this on costs one detail
// request per re-read posting against a host whose crawl fleet is already its own bottleneck,
// so it is enabled deliberately, one provider at a time, with the cost watched. A set value
// must be a positive number of days — an unparseable one fails the run rather than quietly
// crawling as before, which is the same contract every other hand-set knob here has.
//
// BODY_REFRESH_SLICE bounds one run: a posting belongs to one slice for its whole life, and a
// run takes the slice named by the day of the year, so a full sweep takes SLICE days and no
// single run faces a provider's entire catalogue (1.27M postings on workday). Setting it
// without BODY_REFRESH_DAYS is an error rather than a no-op: a knob that silently does nothing
// reads exactly like a refresh that found nothing to refresh.
func bodyRefreshFor(days, slice string, now time.Time) (bodyRefresh, error) {
	if days == "" {
		if slice != "" {
			return bodyRefresh{}, fmt.Errorf("BODY_REFRESH_SLICE=%q has no effect without BODY_REFRESH_DAYS", slice)
		}
		return bodyRefresh{slices: 1, slot: bodyRefreshDisabledSlot}, nil
	}
	n, err := strconv.Atoi(days)
	if err != nil || n <= 0 {
		return bodyRefresh{}, fmt.Errorf("BODY_REFRESH_DAYS must be a positive number of days, got %q", days)
	}
	slices := int64(bodyRefreshDefaultSlices)
	if slice != "" {
		s, err := strconv.Atoi(slice)
		if err != nil || s <= 0 {
			return bodyRefresh{}, fmt.Errorf("BODY_REFRESH_SLICE must be a positive number of runs, got %q", slice)
		}
		slices = int64(s)
	}
	return bodyRefresh{
		cutoff: now.AddDate(0, 0, -n),
		slices: slices,
		slot:   int64(now.YearDay()) % slices,
	}, nil
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

// sweepProvider closes one provider's unseen jobs: the bulk UPDATE (CloseUnseenJobs or
// CloseUnseenJobsBySource) is the fast path, and on error it falls back to sweepRowByRow
// so a single row Postgres can't write doesn't block the rest of the provider — see
// sweepRowByRow for why that happens. skipped is always 0 unless the fallback ran.
func sweepProvider(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, companySlugs []string, bySource bool) (closed int64, skipped int, err error) {
	if bySource {
		closed, err = queries.CloseUnseenJobsBySource(ctx, db.CloseUnseenJobsBySourceParams{
			Source: provider,
			Cutoff: cutoff,
		})
	} else {
		closed, err = queries.CloseUnseenJobs(ctx, db.CloseUnseenJobsParams{
			Source:       provider,
			Cutoff:       cutoff,
			CompanySlugs: companySlugs,
		})
	}
	if err == nil {
		return closed, 0, nil
	}
	log.Printf("close stale jobs (%s): bulk close failed (%v), falling back to row-by-row", provider, err)
	closed, skipped, err = sweepRowByRow(ctx, queries, provider, cutoff, companySlugs, bySource)
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
func sweepRowByRow(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, companySlugs []string, bySource bool) (closed int64, skipped int, err error) {
	var ids []int64
	if bySource {
		ids, err = queries.UnseenJobIDsBySource(ctx, db.UnseenJobIDsBySourceParams{Source: provider, Cutoff: cutoff})
	} else {
		ids, err = queries.UnseenJobIDs(ctx, db.UnseenJobIDsParams{Source: provider, Cutoff: cutoff, CompanySlugs: companySlugs})
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
