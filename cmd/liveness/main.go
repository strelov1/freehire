// Command liveness is the standalone orphan-job liveness worker. It probes the
// posting URL of every open job the ingest sweep never re-crawls — the sources not in
// the ATS provider registry (manual/resolve-url imports and the like), whose closed_at
// would otherwise stay NULL forever — and closes a job once two consecutive probes
// report it dead. (Registered board providers, including aggregators like habr_career
// and geekjob, are swept by cmd/ingest and excluded here — see excluded below. A few
// registered providers whose sweep has a structural blind spot are added back in a
// restricted form — see probeDespiteRegistered — verified by whatever evidence that
// specific source actually offers: jobicy and remoteok candidates go through the same
// plain-GET probe as everything else; himalayas.app 403s a bot-looking GET to any job
// page (Cloudflare), so its candidates are checked against the site's own sitemap
// instead (see himalayas.go); echojobs' stored URL is the employer's own ATS link, not
// echojobs.io's, so its candidates are checked against echojobs.io's own per-posting
// API instead (see echojobs.go). A source with the same leak but NO evidence a probe
// could ever read — see expireDespiteRegistered — falls back to the same age-based
// guess as unsignalledSources instead of a verdict.)
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
	"strings"
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
// close every open job — see job-lifecycle's company_slug-scope leak: each is a
// boardless, recency-ordered aggregator with a fixed crawl budget (page count, offset
// cap, or freshness window) smaller than its live catalogue, so a company whose postings
// age below that budget never re-enters CloseUnseenJobs' crawled-slug scope and its last
// posting stays open forever. These sources are NOT removed from atsProviders — the
// sweep still owns their normal closes — they are only ADDED BACK as liveness
// candidates, restricted to jobs already past the sweep's own staleness window (see
// staleCutoff), so this only ever picks up what the sweep structurally cannot reach
// rather than racing it.
var probeDespiteRegistered = []string{"himalayas", "echojobs", "jobicy", "remoteok"}

// probeDespiteRegisteredGET is the probeDespiteRegistered subset verified by the same
// plain-GET-then-Classify probe as every orphan candidate — their job pages answer a
// normal HTTP status/body, unlike himalayas (Cloudflare-blocked; see himalayas.go) or
// echojobs (the stored URL is the employer's own ATS link, not a page this adapter's own
// site serves; see echojobs.go). Each probeDespiteRegistered member needs its own such
// evidence path decided on a case-by-case basis — there is no generic per-source plugin
// here by design, since a GET probe is not guaranteed to work for any given source.
var probeDespiteRegisteredGET = []string{"jobicy", "remoteok"}

// staleCutoff mirrors cmd/ingest's staleAfter (the sweep's own "unseen" window) via the
// shared sources.DefaultSweepGrace symbol: a probeDespiteRegistered job only becomes a
// liveness candidate once the sweep would already have closed it were the company_slug
// scope not in the way.
const staleCutoff = sources.DefaultSweepGrace

// expireDespiteRegisteredPrefixes lists registered-ATS-provider FAMILIES with the same
// company_slug/keyword-scope leak as probeDespiteRegistered's members, but with NO
// evidence a probe could ever read: see whatjobs.go — jobs.url is the ad network's own
// billing/tracking landing page, not the employer's posting, so it answers the same
// regardless of whether the underlying posting is still live. Unlike probeDespiteRegistered,
// this is the same age-based fallback as unsignalledSources (see expiryWindow) — "what
// cannot be probed is expired instead" — just applied to a source the sweep DOES
// otherwise close on evidence (whatjobs' own extended sweepGrace), for the tail its crawl
// budget structurally can never re-reach.
//
// A prefix, not a source list: whatjobs runs one CPC account PER COUNTRY
// (internal/sources/whatjobs.go's whatjobsMarkets — ~50 as of writing), each its own
// registered provider ("whatjobs" for the bare US market, "whatjobs-<cc>" for every
// other), all sharing the identical unprobeable-URL shape. Matching by prefix against the
// live registry (see the derivation in run()) means a newly onboarded market is covered
// automatically instead of silently falling through an enumerated list.
var expireDespiteRegisteredPrefixes = []string{"whatjobs"}

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

	// Derived from the live registry rather than enumerated (see
	// expireDespiteRegisteredPrefixes) — every whatjobs market this run, no drift guard
	// needed, since membership follows atsProviders by construction.
	expireDespiteRegistered := matchingProviders(atsProviders, expireDespiteRegisteredPrefixes)

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

	// probeDespiteRegistered candidates: restricted to jobs already past the sweep's own
	// staleness window (see staleCutoff) — the sweep's scope leak, not a race with a run
	// still inside its normal grace period. Split by source below into whichever
	// evidence path that source actually has (see probeDespiteRegisteredGET and the
	// per-source apply* functions).
	staleCandidates, err := queries.SelectStaleRegisteredCandidates(ctx, db.SelectStaleRegisteredCandidatesParams{
		Sources: filterSources(probeDespiteRegistered, *sourceFilter),
		Cutoff:  pgtype.Timestamptz{Time: time.Now().Add(-staleCutoff), Valid: true},
	})
	if err != nil {
		log.Printf("select stale registered candidates: %v", err)
		return 1
	}
	var getProbeStale, himalayasStale, echojobsStale []db.SelectStaleRegisteredCandidatesRow
	for _, c := range staleCandidates {
		switch {
		case slices.Contains(probeDespiteRegisteredGET, c.Source):
			getProbeStale = append(getProbeStale, c)
		case c.Source == "himalayas":
			himalayasStale = append(himalayasStale, c)
		case c.Source == "echojobs":
			echojobsStale = append(echojobsStale, c)
		default:
			// Unreachable in practice — every probeDespiteRegistered member is one of
			// the three branches above — but a candidate silently going unverified
			// (rather than erroring loudly) is exactly the class of bug this worker
			// exists to avoid, so a future member added to probeDespiteRegistered
			// without wiring its evidence path here fails visibly instead.
			log.Printf("liveness: %s has no wired evidence path for probeDespiteRegistered — skipping %s", c.Source, c.PublicSlug)
		}
	}
	log.Printf("liveness: %d orphan candidates (excluding %d ATS providers + %d unsignalled sources) + %d stale candidates from %d registered sources (%d GET, %d himalayas, %d echojobs)",
		orphanCount, len(atsProviders), len(unsignalledSources), len(staleCandidates), len(probeDespiteRegistered),
		len(getProbeStale), len(himalayasStale), len(echojobsStale))
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

	// expireDespiteRegistered's same age-based fallback, for the registered providers
	// that need it (whatjobs today) — same query, same window, just a different source
	// list and the reciprocal guard already checked above.
	sourcesToExpireDespiteRegistered := filterSources(expireDespiteRegistered, *sourceFilter)
	expiredRegistered, err := queries.CloseStaleUnsignalledJobs(ctx, db.CloseStaleUnsignalledJobsParams{
		Sources: sourcesToExpireDespiteRegistered,
		Cutoff:  pgtype.Timestamptz{Time: time.Now().Add(-expiryWindow), Valid: true},
	})
	if err != nil {
		log.Printf("liveness: expire stale expireDespiteRegistered jobs: %v", err)
	} else {
		log.Printf("liveness: expired %d jobs posted more than %d days ago from %d expireDespiteRegistered sources",
			expiredRegistered, int(expiryWindow.Hours()/24), len(sourcesToExpireDespiteRegistered))
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
			probeAndApply(ctx, client, queries, c.ID, c.PublicSlug, c.Source, c.URL, c.LivenessStrikes, &probed, &closed, &struck, &failed)
		}(c)
	}
	wg.Wait()

	// probeDespiteRegisteredGET candidates (jobicy, remoteok today): same plain-GET
	// probe as the orphan loop above, just a different candidate source.
	for _, c := range getProbeStale {
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.SelectStaleRegisteredCandidatesRow) {
			defer wg.Done()
			defer func() { <-sem }()
			probeAndApply(ctx, client, queries, c.ID, c.PublicSlug, c.Source, c.URL, c.LivenessStrikes, &probed, &closed, &struck, &failed)
		}(c)
	}
	wg.Wait()

	// probeDespiteRegistered's own dedicated evidence sources — see the package doc and
	// probeDespiteRegisteredGET for why these two can't share the GET-probe loop above.
	applyHimalayasVerdicts(ctx, client, queries, himalayasStale, sem, &probed, &closed, &struck, &failed)
	applyEchoJobsVerdicts(ctx, client, queries, echojobsStale, sem, &probed, &closed, &struck, &failed)

	log.Printf("liveness done: probed=%d closed=%d struck=%d failed=%d", probed, closed, struck, failed)
	return worker.ExitCode(int(failed), 0)
}

// probeAndApply GETs url, classifies the response via liveness.Classify, and applies the
// resulting verdict — the plain-HTTP-probe path shared by every orphan candidate and the
// probeDespiteRegisteredGET subset of registered-provider candidates.
func probeAndApply(ctx context.Context, client *http.Client, queries *db.Queries, id int64, publicSlug, source, url string, livenessStrikes int32, probed, closed, struck, failed *int64) {
	status, finalURL, body, ferr := liveness.Fetch(ctx, client, url)
	if ferr != nil {
		// A probe that could not reach the page is Uncertain (status 0), so
		// applyVerdict takes no action — a fetch failure never advances or clears a
		// strike, and is not counted as a worker failure.
		log.Printf("liveness: probe %s failed: %v", publicSlug, ferr)
	}
	verdict, reason := liveness.Classify(status, finalURL, body)
	applyVerdict(ctx, queries, id, publicSlug, source, livenessStrikes, verdict, reason, probed, closed, struck, failed)
}

// applyVerdict applies a liveness verdict to one candidate: advances/closes on Expired,
// clears strikes on Live, and leaves Uncertain untouched. Shared by every evidence path
// in this worker (plain-GET probe, himalayas sitemap membership, echojobs detail API) so
// the close/strike/reset semantics live in exactly one place. Takes the candidate's
// fields directly rather than a query row type, since its callers draw from more than
// one sqlc row shape (SelectOrphanLivenessCandidatesRow, SelectStaleRegisteredCandidatesRow).
func applyVerdict(ctx context.Context, queries *db.Queries, id int64, publicSlug, source string, livenessStrikes int32, verdict liveness.Verdict, reason string, probed, closed, struck, failed *int64) {
	atomic.AddInt64(probed, 1)
	switch verdict {
	case liveness.Expired:
		res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{
			ID:        id,
			Threshold: closeThreshold,
		})
		if err != nil {
			// The verdict was reached but the DB update did not apply — a real
			// failure the exit code must surface, not a silent log-and-continue.
			atomic.AddInt64(failed, 1)
			log.Printf("liveness: mark expired %s: %v", publicSlug, err)
			return
		}
		if res.ClosedAt.Valid {
			atomic.AddInt64(closed, 1)
			log.Printf("liveness: closed %s (%s, %s)", publicSlug, source, reason)
		} else {
			atomic.AddInt64(struck, 1)
			log.Printf("liveness: strike %d/%d %s (%s)", res.LivenessStrikes, closeThreshold, publicSlug, reason)
		}
	case liveness.Live:
		// Clear any accumulated strikes. Skip the write when there is nothing to
		// clear so a healthy catalogue does not issue an UPDATE per open job.
		if livenessStrikes != 0 {
			if err := queries.ResetLivenessStrikes(ctx, id); err != nil {
				atomic.AddInt64(failed, 1)
				log.Printf("liveness: reset %s: %v", publicSlug, err)
			}
		}
	case liveness.Uncertain:
		// No signal either way — leave the strike count untouched.
	}
}

// applyHimalayasVerdicts verifies every himalayas probeDespiteRegistered candidate
// against the site's own sitemap of what is currently live — a plain GET of the job page
// itself is Cloudflare-blocked (see himalayas.go), so this is the source's only available
// evidence. A sitemap fetch failure counts as one worker failure and skips every
// candidate this run: under-closing (leaving them open another run) is the only
// acceptable outcome of not being able to verify, matching the bias everywhere else in
// this worker.
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
			applyVerdict(ctx, queries, c.ID, c.PublicSlug, c.Source, c.LivenessStrikes, verdict, reason, probed, closed, struck, failed)
		}(c)
	}
	wg.Wait()
}

// applyEchoJobsVerdicts verifies every echojobs probeDespiteRegistered candidate against
// echojobs.io's own per-posting detail API — see echojobs.go for why the stored jobs.url
// (the employer's own ATS link, not an echojobs.io page) is not what gets probed.
func applyEchoJobsVerdicts(ctx context.Context, client *http.Client, queries *db.Queries, candidates []db.SelectStaleRegisteredCandidatesRow, sem chan struct{}, probed, closed, struck, failed *int64) {
	var wg sync.WaitGroup
	for _, c := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.SelectStaleRegisteredCandidatesRow) {
			defer wg.Done()
			defer func() { <-sem }()
			verdict, reason := checkEchoJobsLive(ctx, client, c.ExternalID)
			applyVerdict(ctx, queries, c.ID, c.PublicSlug, c.Source, c.LivenessStrikes, verdict, reason, probed, closed, struck, failed)
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

// matchingProviders returns every entry of providers that equals one of prefixes or has
// it as a "<prefix>-" prefix — how a provider family sharing one adapter across many
// per-market registry keys (e.g. whatjobs' one row per country, see
// expireDespiteRegisteredPrefixes) is matched without enumerating every key.
func matchingProviders(providers, prefixes []string) []string {
	var out []string
	for _, p := range providers {
		for _, prefix := range prefixes {
			if p == prefix || strings.HasPrefix(p, prefix+"-") {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
