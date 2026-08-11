// Command liveness is the standalone orphan-job liveness worker. It probes the
// posting URL of every open job the ingest sweep never re-crawls — the sources not in
// the ATS provider registry (manual/resolve-url imports and the like), whose closed_at
// would otherwise stay NULL forever — and closes a job once two consecutive probes
// report it dead. (Registered board providers, including aggregators like habr_career
// and geekjob, are swept by cmd/ingest and excluded here — see excluded below. A few
// registered providers whose sweep has a structural blind spot are added back in a
// restricted form — see probeDespiteRegistered — verified by whatever evidence that
// specific source actually offers, not necessarily a plain-HTTP probe of its own page:
// himalayas.app 403s a bot-looking GET to any job page (Cloudflare), so its candidates
// are instead checked against the site's own sitemap — see himalayas.go.)
//
// It is a run-once-and-exit worker (cron-scheduled beside ingest/enrich): select
// candidates, probe each over plain HTTP, classify, apply the strike/close/reset
// update, and exit. Re-running is safe; only a definitive death signal confirmed
// twice in a row closes a job, biasing toward leaving orphans open over a false
// close (orphans have no re-ingest to reopen them). It exits non-zero when a probe
// could not apply its DB update, so cron can alert.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/liveness"
	"github.com/strelov1/freehire/internal/safehttp"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

const (
	// closeThreshold is the number of CONSECUTIVE expired probes that closes a job.
	// Two reads across separate runs absorb a transient death signal (an employer
	// site mid-deploy) without a probe-history table.
	closeThreshold = 2
	// probeTimeout bounds a single URL fetch so one slow host cannot stall the run.
	probeTimeout = 15 * time.Second
	// concurrency caps simultaneous probes: orphan postings span many hosts, so this
	// keeps the worker from hammering any single employer site while staying brisk.
	concurrency = 8
	// lockKey is the Postgres advisory-lock key that serializes liveness runs. Cron
	// offers no host-level guarantee against stacking, and two runs probing the same
	// orphan seconds apart would collapse the "two consecutive expired reads" grace
	// into one burst — closing a job on a transient blip. A second run that can't take
	// the lock exits cleanly. The value is an arbitrary constant unique to this worker.
	lockKey = 0x66686c76 // "fhlv" — freehire liveness; the key list lives in internal/migrate
)

// unsignalledSources carry no close signal at all: no re-crawl that could stop seeing
// them, no change feed, and a stored URL that is a container outliving the vacancy rather
// than the vacancy's own page — so a liveness probe can never reach a death verdict. Only
// telegram qualifies today: its URL is the Telegram post (see cmd/tg-extract), which stays
// live after the job is filled. (habr_career/geekjob are already excluded as registered
// providers — cmd/ingest's sweep owns their closes.)
//
// This one list plays both roles, and they are two halves of the same decision: these
// sources are excluded from the probe BECAUSE probing them is futile, and closed by age
// INSTEAD. Keeping it single means a source can never be dropped from the probe without
// something else taking over its closes.
var unsignalledSources = []string{"telegram"}

// expiryWindow is how old a posting from an unsignalledSource must be before it is
// presumed filled. Deliberately generous: this is the only close in the lifecycle that
// rests on a guess rather than on evidence, so it takes the same under-closing bias the
// probe does. Measured on the catalogue at 45 days it closes the tail without taking the
// bulk of postings still inside a normal hiring cycle.
const expiryWindow = 45 * 24 * time.Hour

// probeDespiteRegistered lists registered ATS providers whose ingest sweep still can't
// close every open job — see job-lifecycle's company_slug-scope leak. himalayas pages
// only its freshest slice per run (himalayasMaxPages), so a company that ages out of
// that recency window never re-enters CloseUnseenJobs' crawled-slug scope and its last
// posting stays open forever. These sources are NOT removed from atsProviders — the
// sweep still owns their normal closes — they are only ADDED BACK as liveness
// candidates, restricted to jobs already past the sweep's own staleness window (see
// staleCutoff), so this only ever picks up what the sweep structurally cannot reach
// rather than racing it. Each member needs its own way to actually get evidence (a
// plain GET of the job page is not guaranteed to work — himalayas's is Cloudflare-
// blocked, so its candidates are verified against the site's sitemap instead; see
// himalayas.go), which is also why this list is not expected to grow casually.
var probeDespiteRegistered = []string{"himalayas"}

// staleCutoff mirrors cmd/ingest's staleAfter (the sweep's own "unseen" window): a
// probeDespiteRegistered job only becomes a liveness candidate once the sweep would
// already have closed it were the company_slug scope not in the way.
const staleCutoff = 48 * time.Hour

func main() {
	worker.Main(run)
}

func run() int {
	// sourceFilter restricts a manual run to one source (e.g. `-source=himalayas`) —
	// cron never sets it, so the default run probes every eligible source as before.
	// Meant for debugging a single adapter's close behaviour on prod without probing
	// (or age-expiring) the rest of the catalogue in the same run.
	sourceFilter := flag.String("source", "", "restrict this run to one source, for debugging (empty = every eligible source)")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)

	// Single-flight: hold a session-scoped advisory lock on a dedicated connection
	// for the whole run so overlapping cron invocations can't strike the same orphan
	// twice within one burst. A run that can't take the lock exits cleanly.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("acquire lock connection: %v", err)
		return 1
	}
	var locked bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(lockKey)).Scan(&locked); err != nil {
		lockConn.Release()
		log.Printf("liveness lock: %v", err)
		return 1
	}
	if !locked {
		lockConn.Release()
		log.Print("liveness: another run holds the lock — exiting")
		return 0
	}
	defer func() {
		// Best-effort unlock; releasing/closing the connection drops the session lock anyway.
		_, _ = lockConn.Exec(ctx, "SELECT pg_advisory_unlock($1)", int64(lockKey))
		lockConn.Release()
	}()

	// The candidate set is "every open job whose source is not a registered ATS
	// provider" — the registry keys are the exclusion list, so a new adapter never
	// silently becomes a probe target.
	atsProviders := providerKeys(sources.All(sources.NewClient()))
	// Guard: `source <> ALL('{}')` is vacuously TRUE in Postgres, so an empty
	// exclusion list would select EVERY open job — including board jobs the ingest
	// sweep owns. Refuse to run rather than risk URL-closing the whole catalogue.
	if len(atsProviders) == 0 {
		log.Print("liveness: no ATS providers registered — refusing to run (would probe every open job)")
		return 1
	}

	// Guard: an unsignalledSource must not be a source some other mechanism already
	// closes on evidence. The age rule closes on a guess, so overlapping the ingest
	// sweep would let a guess override evidence — and it would do so silently, since
	// the probe exclusion for such a source is a no-op (it is excluded as a provider
	// anyway). Refuse to run rather than mass-close a swept provider by age.
	for _, s := range unsignalledSources {
		if slices.Contains(atsProviders, s) {
			log.Printf("liveness: %q is a registered ATS provider — refusing to run (the sweep owns its closes; age must not override evidence)", s)
			return 1
		}
	}

	// Guard: the inverse of the check above — probeDespiteRegistered exists to add
	// candidates BACK for sources the exclusion above just removed, so an entry that
	// falls out of the registry (renamed/retired adapter) would silently become a
	// no-op rather than an error. Refuse to run so the drift gets noticed.
	for _, s := range probeDespiteRegistered {
		if !slices.Contains(atsProviders, s) {
			log.Printf("liveness: %q is not a registered ATS provider — refusing to run (probeDespiteRegistered is stale)", s)
			return 1
		}
	}

	// Appended AFTER the guard above so the empty-ATS-registry safeguard still keys
	// off atsProviders alone. See unsignalledSources for why these are excluded.
	excluded := append(atsProviders, unsignalledSources...)

	candidates, err := queries.SelectOrphanLivenessCandidates(ctx, excluded)
	if err != nil {
		log.Printf("select candidates: %v", err)
		return 1
	}
	if *sourceFilter != "" {
		// SelectOrphanLivenessCandidates has no source allowlist of its own (it is
		// "everything not a registered provider"), so -source is applied here rather
		// than pushed into the query.
		candidates = slices.DeleteFunc(candidates, func(c db.SelectOrphanLivenessCandidatesRow) bool {
			return c.Source != *sourceFilter
		})
	}
	orphanCount := len(candidates)

	// probeDespiteRegistered candidates: restricted to jobs already past the sweep's
	// own staleness window (see staleCutoff) — the sweep's scope leak, not a race
	// with a run still inside its normal grace period. Verified separately below
	// (sitemap membership, not a GET probe) — see applyHimalayasVerdicts.
	staleCandidates, err := queries.SelectStaleRegisteredCandidates(ctx, db.SelectStaleRegisteredCandidatesParams{
		Sources: filterSources(probeDespiteRegistered, *sourceFilter),
		Cutoff:  pgtype.Timestamptz{Time: time.Now().Add(-staleCutoff), Valid: true},
	})
	if err != nil {
		log.Printf("select stale registered candidates: %v", err)
		return 1
	}
	log.Printf("liveness: %d orphan candidates (excluding %d ATS providers + %d unsignalled sources) + %d stale candidates from %d registered sources",
		orphanCount, len(atsProviders), len(unsignalledSources), len(staleCandidates), len(probeDespiteRegistered))
	if *sourceFilter != "" {
		log.Printf("liveness: restricted to source %q for this run", *sourceFilter)
	}

	// The other half of the same decision: what the probe cannot judge, age judges.
	// Run before probing so a run that dies partway through the probe still expires.
	// filterSources means a -source run that targets neither unsignalledSources member
	// expires nothing this run, rather than age-closing telegram jobs on a debug probe
	// of an unrelated source.
	sourcesToExpire := filterSources(unsignalledSources, *sourceFilter)
	expired, err := queries.CloseStaleUnsignalledJobs(ctx, db.CloseStaleUnsignalledJobsParams{
		Sources: sourcesToExpire,
		Cutoff:  pgtype.Timestamptz{Time: time.Now().Add(-expiryWindow), Valid: true},
	})
	if err != nil {
		// Log and carry on rather than return: the guess-half failing must not disable
		// the evidence-half. A lock wait against a concurrent reindex would otherwise
		// stop orphan probing on every subsequent cron run, silently and indefinitely.
		log.Printf("liveness: expire stale unsignalled jobs: %v", err)
	} else {
		log.Printf("liveness: expired %d jobs posted more than %d days ago from %d unsignalled sources",
			expired, int(expiryWindow.Hours()/24), len(sourcesToExpire))
	}

	// Probe targets are orphan-job URLs that originated from attacker-influenced
	// sources (telegram posts), so the probe must refuse internal/metadata targets.
	client := safehttp.NewClient(probeTimeout)
	var probed, closed, struck, failed int64
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for _, c := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.SelectOrphanLivenessCandidatesRow) {
			defer wg.Done()
			defer func() { <-sem }()

			status, finalURL, body, ferr := liveness.Fetch(ctx, client, c.URL)
			if ferr != nil {
				// A probe that could not reach the page is Uncertain (status 0), so
				// applyVerdict takes no action — a fetch failure never advances or
				// clears a strike, and is not counted as a worker failure.
				log.Printf("liveness: probe %s failed: %v", c.PublicSlug, ferr)
			}
			verdict, reason := liveness.Classify(status, finalURL, body)
			applyVerdict(ctx, queries, c, verdict, reason, &probed, &closed, &struck, &failed)
		}(c)
	}
	wg.Wait()

	// probeDespiteRegistered's own evidence source (sitemap membership for himalayas
	// today — see himalayas.go), applied with the same strike/close/reset semantics as
	// the GET probe above via applyVerdict, just fed a different verdict.
	applyHimalayasVerdicts(ctx, client, queries, staleCandidates, sem, &probed, &closed, &struck, &failed)

	log.Printf("liveness done: probed=%d closed=%d struck=%d failed=%d", probed, closed, struck, failed)
	return worker.ExitCode(int(failed), 0)
}

// applyVerdict applies a liveness verdict to one candidate: advances/closes on Expired,
// clears strikes on Live, and leaves Uncertain untouched. Shared by the GET-probe loop
// (verdict from liveness.Classify) and applyHimalayasVerdicts (verdict from sitemap
// membership) so the close/strike/reset semantics live in exactly one place.
func applyVerdict(ctx context.Context, queries *db.Queries, c db.SelectOrphanLivenessCandidatesRow, verdict liveness.Verdict, reason string, probed, closed, struck, failed *int64) {
	atomic.AddInt64(probed, 1)
	switch verdict {
	case liveness.Expired:
		res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{
			ID:        c.ID,
			Threshold: closeThreshold,
		})
		if err != nil {
			// The verdict was reached but the DB update did not apply — a real
			// failure the exit code must surface, not a silent log-and-continue.
			atomic.AddInt64(failed, 1)
			log.Printf("liveness: mark expired %s: %v", c.PublicSlug, err)
			return
		}
		if res.ClosedAt.Valid {
			atomic.AddInt64(closed, 1)
			log.Printf("liveness: closed %s (%s, %s)", c.PublicSlug, c.Source, reason)
		} else {
			atomic.AddInt64(struck, 1)
			log.Printf("liveness: strike %d/%d %s (%s)", res.LivenessStrikes, closeThreshold, c.PublicSlug, reason)
		}
	case liveness.Live:
		// Clear any accumulated strikes. Skip the write when there is nothing to
		// clear so a healthy catalogue does not issue an UPDATE per open job.
		if c.LivenessStrikes != 0 {
			if err := queries.ResetLivenessStrikes(ctx, c.ID); err != nil {
				atomic.AddInt64(failed, 1)
				log.Printf("liveness: reset %s: %v", c.PublicSlug, err)
			}
		}
	case liveness.Uncertain:
		// No signal either way — leave the strike count untouched.
	}
}

// applyHimalayasVerdicts verifies every probeDespiteRegistered candidate (today: only
// himalayas) against the site's own sitemap of what is currently live — a plain GET of
// the job page itself is Cloudflare-blocked (see himalayas.go), so this is the source's
// only available evidence. A sitemap fetch failure counts as one worker failure and
// skips every candidate this run: under-closing (leaving them open another run) is the
// only acceptable outcome of not being able to verify, matching the bias everywhere
// else in this worker.
func applyHimalayasVerdicts(ctx context.Context, client *http.Client, queries *db.Queries, candidates []db.SelectStaleRegisteredCandidatesRow, sem chan struct{}, probed, closed, struck, failed *int64) {
	if len(candidates) == 0 {
		return
	}
	liveURLs, err := fetchHimalayasLiveJobURLs(ctx, client)
	if err != nil {
		atomic.AddInt64(failed, 1)
		log.Printf("liveness: himalayas sitemap fetch failed, skipping %d stale registered candidates: %v", len(candidates), err)
		return
	}

	var wg sync.WaitGroup
	for _, c := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.SelectStaleRegisteredCandidatesRow) {
			defer wg.Done()
			defer func() { <-sem }()
			verdict, reason := liveness.Live, ""
			if _, ok := liveURLs[c.URL]; !ok {
				verdict, reason = liveness.Expired, "absent_from_sitemap"
			}
			applyVerdict(ctx, queries, db.SelectOrphanLivenessCandidatesRow(c), verdict, reason, probed, closed, struck, failed)
		}(c)
	}
	wg.Wait()
}

// providerKeys returns the registered ATS provider keys — the sources the ingest
// sweep owns and the liveness probe must exclude.
func providerKeys(registry map[string]sources.Source) []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}

// filterSources narrows a source list to sourceFilter for a debug run (-source=X);
// an empty sourceFilter returns list unchanged, and a filter absent from list returns
// nil rather than list, so a run scoped to a source outside this list touches none of it.
func filterSources(list []string, sourceFilter string) []string {
	if sourceFilter == "" {
		return list
	}
	if slices.Contains(list, sourceFilter) {
		return []string{sourceFilter}
	}
	return nil
}
