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
	// The provider to crawl. Accept it as the first positional argument (cron passes the
	// provider name) or via INGEST_PROVIDER. An optional --shard=i/n (or the SHARD env)
	// crawls only a round-robin slice of that provider's boards, so a provider with too
	// many boards to finish in one timeout (workday) is spread across several staggered
	// runs.
	var provider, shardSpec string
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--shard="):
			shardSpec = strings.TrimPrefix(a, "--shard=")
		case a == "--shard":
			// The value must be attached (--shard=i/n); a space-separated form would
			// otherwise swallow the next arg (the provider name) as the selector's value.
			log.Print("config: --shard needs an attached value, e.g. --shard=2/6")
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
	store := newDBStore(pool, enrich.Version, crawled, tally, hydrationWindow, refetchAll)
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

	runStats, err := runner.Run(ctx, sourceCfg.Sources)
	if err != nil {
		log.Printf("ingest: %v", err)
		return 1
	}

	// Surface which boards are failing / cooled without grepping logs — one line
	// naming them, their failure count, and when they next become eligible.
	logUnhealthyBoards(ctx, db.New(pool))

	total := runStats.Total()
	log.Printf("ingest done: provider=%s providers=%d ingested=%d failed=%d skipped=%d rejected=%d",
		provider, len(runStats), total.Ingested, total.Failed, total.Skipped, total.Rejected)

	// A provider at 0% took the cheap write for nothing all run — a hashed field is churning
	// between crawls, which costs a full row rewrite AND a pointless index push every pass.
	if s := tally.summary(); s != "" {
		log.Printf("ingest writes: provider=%s %s", provider, s)
	}

	// A failed board is counted in total.Failed; surface it (and any sweep failure
	// below) through the exit code so cron alerts on a degraded run.
	failed := total.Failed

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
	queries := db.New(pool)
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
		bySource := sweepBySource(runStats[provider], fullCatalog[provider])
		companySlugs := crawled.slugs(provider)

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

		// Board-scoped close (job-lifecycle spec, freehire#2328): the company scope above
		// leaks a company whose LAST posting drops off a board this run still crawled — that
		// company's slug never re-enters companySlugs, so its row never closes under either
		// scope so far. This closes it directly, per board the run structurally proved it
		// covered (runStats[provider].QualifyingBoards) on a provider whose adapter is
		// registered as listing a board to completion — see sweepableBoards for the full gate.
		// Reuses this provider's own cutoff: a board-scope candidate is by construction never
		// a sweepGrace provider, so the window is always the default here.
		var boardFailed int
		for _, boardID := range sweepableBoards(runStats[provider], hasGrace, fullCatalog[provider], fullBoardListing[provider], crossShardAmbiguousBoards) {
			boardClosed, err := queries.CloseUnseenJobsForBoard(ctx, db.CloseUnseenJobsForBoardParams{
				Source:       provider,
				Cutoff:       cutoff,
				BoardPattern: externalid.BoardPattern(boardID),
			})
			if err != nil {
				// One board's failure must not skip the rest — see sweepProvider's own
				// per-provider isolation for the same reasoning, one scope narrower.
				boardFailed++
				log.Printf("close stale jobs (%s board %q): %v", provider, boardID, err)
				continue
			}
			if boardClosed > 0 {
				log.Printf("closed %d stale %s jobs on board %q (unseen for %s)", boardClosed, provider, boardID, window)
			}
		}
		if boardFailed > 0 {
			failed++
		}
	}
	return worker.ExitCode(failed, 0)
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
func sweepBySource(stats pipeline.Stats, fullCatalog bool) bool {
	return fullCatalog && stats.Failed == 0
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
