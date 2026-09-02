// Command ingest is the standalone source-ingest worker. It loads one board file
// (sources/<provider>.yml, or a mixed sources/custom.yml — passed as the first argument
// or via SOURCES_FILE), fetches each board through its adapter, normalizes the postings,
// and upserts them — enqueuing new ones for enrichment in the same write. A file's
// entries usually share the file-name provider, but an entry may name its own, so one
// file can cover several single-source providers. After the run each provider that
// ingested at least one job has its stale jobs swept. Run on a schedule (e.g. cron); it
// processes its boards once and exits.
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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// staleAfter is the DEFAULT grace window before an unseen job is closed. An adapter that
// crawls only a slice of its catalogue widens it for its own provider — see sweepWindowFor.
// Shared with cmd/liveness's staleCutoff via sources.DefaultSweepGrace, since liveness's
// probeDespiteRegistered backstop only picks up what this sweep has already had a chance
// to close and must not drift out of sync with it.
const staleAfter = sources.DefaultSweepGrace

func main() {
	worker.Main(run)
}

func run() int {
	// The board file is usually one provider's list (sources/<provider>.yml, provider =
	// file name), but an entry may name its own provider, so it may be a mixed file
	// (sources/custom.yml). Accept it as the first positional argument (cron passes the
	// file) or via SOURCES_FILE. An optional --shard=i/n (or the SHARD env) crawls only a
	// round-robin slice of the file, so a provider with too many boards to finish in one
	// timeout (workday) is spread across several staggered runs.
	var path, shardSpec string
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--shard="):
			shardSpec = strings.TrimPrefix(a, "--shard=")
		case a == "--shard":
			// The value must be attached (--shard=i/n); a space-separated form would
			// otherwise swallow the next arg (the board path) as the selector's value.
			log.Print("config: --shard needs an attached value, e.g. --shard=2/6")
			return 1
		case a != "" && !strings.HasPrefix(a, "-") && path == "":
			path = a
		}
	}
	if path == "" {
		path = os.Getenv("SOURCES_FILE")
	}
	if shardSpec == "" {
		shardSpec = os.Getenv("SHARD")
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
	// Route the IP-blocklisted providers through the egress proxy when one is configured
	// (SOURCES_PROXY_URL). No-op when unset; a set-but-invalid value fails the run here,
	// before the DB is touched.
	if err := sources.ApplyProxyEgress(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	// Fail fast before touching the DB: a misconfigured board should not start a run.
	// Validate the WHOLE file (not just this shard's slice) so a config error anywhere is
	// caught on every shard's run, not only when its shard happens to include the bad entry.
	if err := sourceCfg.Validate(registry); err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// Narrow to this shard's slice, if requested. Applied after validation, so the run
	// crawls only its share of a large board file while the stale-job sweep below stays
	// safe (it already scopes closes to the companies actually crawled this run).
	if shardSpec != "" {
		i, n, err := sources.ParseShard(shardSpec)
		if err != nil {
			log.Printf("config: %v", err)
			return 1
		}
		full := len(sourceCfg.Sources)
		sourceCfg = sourceCfg.Shard(i, n)
		log.Printf("ingest: shard %d/%d — crawling %d of %d boards in %s", i, n, len(sourceCfg.Sources), full, path)
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

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// crawled records the company slugs each provider actually wrote this run, so the
	// post-run sweep scopes its closes to them (see the sweep below and crawledSet).
	crawled := newCrawledSet()
	// tally records which of the two writes each persisted posting took, so the run says how far
	// the cheap path actually reached rather than leaving it to be assumed (see writeTally).
	tally := newWriteTally()
	store := newDBStore(pool, enrich.Version, crawled, tally, hydrationWindow)
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
	log.Printf("ingest done: file=%s providers=%d ingested=%d failed=%d skipped=%d rejected=%d",
		path, len(runStats), total.Ingested, total.Failed, total.Skipped, total.Rejected)

	// A provider at 0% took the cheap write for nothing all run — a hashed field is churning
	// between crawls, which costs a full row rewrite AND a pointless index push every pass.
	if s := tally.summary(); s != "" {
		log.Printf("ingest writes: file=%s %s", path, s)
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
	// the default window would close it and reopen it on the next run.
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
	for _, provider := range sweepableProviders(runStats) {
		if selfClosing[provider] {
			continue
		}
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
// them: the window its adapter declared (sources.SweepGraceWindows) when it crawls only a slice
// of a catalogue too deep to walk, else staleAfter. Widening is per-provider so one feed's drift
// tolerance never slows the sweep for the ATS boards, whose crawls are complete.
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
