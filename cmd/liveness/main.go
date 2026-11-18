// Command liveness is the standalone orphan-job liveness worker. It probes the
// posting URL of every open job the ingest sweep never re-crawls (sources not in the
// ATS provider registry, such as manual and resolve-url imports) and closes a job once
// two consecutive probes report it dead.
//
// Registered board providers are swept by cmd/ingest and excluded (see excluded in
// run). Providers whose sweep has a structural blind spot are added back in a
// restricted form (probeDespiteRegistered), each verified by the evidence that source
// offers: a plain GET, a sitemap (himalayas.go), a per-posting API (echojobs.go,
// fourdayweek.go, jobtech.go) or a paced page read (hh.go, djinni.go, torre.go,
// arbeitnow.go). A source with the same leak but no readable evidence
// (expireDespiteRegistered) falls back to an age-based guess instead of a verdict.
//
// It is a run-once-and-exit worker: select candidates, probe each, apply the
// strike/close/reset update, and exit. Re-running is safe. Only a definitive death
// signal confirmed twice in a row closes a job, so orphans (which have no re-ingest to
// reopen them) stay open rather than risk a false close. It exits non-zero when a
// probe could not apply its DB update, so cron can alert.
// could not apply its DB update, so cron can alert.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/safehttp"
	"github.com/strelov1/freehire/internal/platform/worker"
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
	lockKey = 0x66686c76 // "fhlv" — freehire liveness; the key list lives in internal/platform/migrate
)

// unsignalledSources carry no close signal at all: no re-crawl that could stop seeing
// them, no change feed, and a stored URL that is a container outliving the vacancy rather
// than its own page, so a probe can never reach a death verdict. Only telegram qualifies:
// its URL is usually the Telegram post, which stays live after the job is filled, and an
// apply link it names is mostly LinkedIn, which answers a bot with a login wall.
//
// This one list is both the probe exclusion and the age-close set, so a source can never
// be dropped from the probe without something else taking over its closes.
var unsignalledSources = []string{"telegram"}

// expiryWindow is how old an unsignalledSource posting must be before it is presumed
// filled. Generous on purpose: this is the only close that rests on a guess, so it takes
// the same under-closing bias as the probe.
const expiryWindow = 45 * 24 * time.Hour

// probeDespiteRegistered lists registered ATS providers whose ingest sweep still can't
// close every open job: each is a recency-ordered aggregator whose crawl budget (page
// count, offset cap or freshness window) is smaller than its live catalogue, so a company
// whose postings age out of it never re-enters CloseUnseenJobs' crawled-slug scope and
// its last posting stays open forever. They stay in atsProviders (the sweep still owns
// their normal closes) and are only added back as liveness candidates, restricted to jobs
// past the sweep's own staleness window (staleCutoff).
//
// Per-source leaks: empregos reads only postings that name a state and cannot prove it
// read the rest; djinni lists only the last month, arbeitnow the last week; 4dayweek
// drops an employer's company from the sweep scope when it leaves the feed; torre's
// sitemap still names some closed postings; jobtech is never swept, it closes from its
// change feed, and a removal missed inside the feed's 72h window is never resent.
var probeDespiteRegistered = []string{"himalayas", "echojobs", "jobicy", "remoteok", "hh", "empregos", "djinni", "4dayweek", "torre", "arbeitnow", "jobtech", "instaffo", "remotedotcom", "thehub", "reed", "arbeitsagentur", "xing", "gradcracker", "engage", "betterteam", "linkedin"}

// probeDespiteRegisteredGET is the probeDespiteRegistered subset verified by the plain
// GET-then-Classify probe. Every other member needs its own evidence path, decided case by
// case: there is no generic per-source plugin, since a GET is not guaranteed to work.
var probeDespiteRegisteredGET = []string{"jobicy", "remoteok", "empregos"}

// pacedPasses are the probeDespiteRegistered members probed one request at a time rather than
// on the fan-out: each meters the host the crawl reads it from (see paced.go), and each reads
// its own evidence — hh's archived flag, djinni's page through the shared Classify, 4dayweek's
// per-posting API, torre's page state, arbeitnow's status, jobtech's per-ad API.
func pacedPasses(client *http.Client) []pacedPass {
	return append([]pacedPass{
		hhPass(client, hhVacancyPage),
		djinniPass(client),
		fourDayWeekPass(client, fourDayWeekJobAPI),
		torrePass(client, torrePostPage),
		arbeitnowPass(client),
		jobtechPass(client, jobtechAdAPI),
		linkedInPass(client),
	}, catalogStatusPasses(client)...)
}

// staleCutoff mirrors cmd/ingest's staleAfter (the sweep's own "unseen" window) via the
// shared sources.DefaultSweepGrace symbol: a probeDespiteRegistered job only becomes a
// liveness candidate once the sweep would already have closed it were the company_slug
// scope not in the way.
const staleCutoff = sources.DefaultSweepGrace

// expireDespiteRegisteredPrefixes lists registered-provider FAMILIES with the same
// company_slug/keyword-scope leak as probeDespiteRegistered but no evidence a probe can
// read. whatjobs' jobs.url is the ad network's billing landing page (see whatjobs.go);
// adzuna's stored `/land/ad/<id>` tracking redirect answers 403 to a bot, which classifies
// uncertain, so this worker can never close an adzuna posting (cmd/adzuna-liveness reads
// the hosted page, and this age rule is the backstop). Both also lose postings from a
// date-ordered, bounded crawl that never revisits an ageing one.
//
// Unlike unsignalledSources, these are re-crawled, so the age guess must agree with what
// the crawl last saw before it may close anything (CloseStaleUnseenUnprobeableJobs and
// unseenWindow). Sharing the telegram rule closed postings the crawl still listed and the
// next crawl reopened them.
//
// A prefix, not a list: whatjobs runs one registered provider per country ("whatjobs" for
// the US, "whatjobs-<cc>" otherwise), so a new market is covered automatically.
var expireDespiteRegisteredPrefixes = []string{"whatjobs", "adzuna"}

func main() {
	worker.Main(run)
}

func run() int {
	// sourceFilter restricts a manual run to one source (e.g. `-source=himalayas`) —
	// cron never sets it, so the default run probes every eligible source as before.
	// Meant for debugging a single adapter's close behaviour on prod without probing
	// (or age-expiring) the rest of the catalogue in the same run.
	sourceFilter := flag.String("source", "", "restrict this run to one source, for debugging (empty = every eligible source)")
	jobIDsFlag := flag.String("job-ids", "", "probe only these comma-separated IDs (max 500); skips all bulk expiry")
	flag.Parse()
	explicitJobIDs := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "job-ids" {
			explicitJobIDs = true
		}
	})
	if explicitJobIDs && *jobIDsFlag == "" {
		log.Print("liveness: explicit job-ids must not be empty")
		return 1
	}
	jobIDs, err := parseProbeJobIDs(*jobIDsFlag)
	if err != nil {
		log.Printf("liveness: %v", err)
		return 1
	}

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
	// provider" — the taxonomy keys are the exclusion list, so a new adapter never
	// silently becomes a probe target.
	registry := sources.All(sources.NewClient())
	crawlProviders := providerKeys(registry)
	atsProviders := classifiedProviderKeys()
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

	// Keep the credential guard on the live crawl registry. The fallback's membership
	// and grace come from taxonomy, so a market missing this host's credentials still
	// has an age-based closure path instead of being excluded from every path.
	if missing := unmatchedPrefixes(crawlProviders, expireDespiteRegisteredPrefixes); len(missing) > 0 {
		log.Printf("liveness: %q matches no registered ATS provider — refusing to run (a source that cannot be age-expired would silently fall to the URL probe instead)",
			strings.Join(missing, ", "))
		return 1
	}
	expireDespiteRegistered, registeredSweepGrace := registeredAgeFallback()

	// Appended AFTER the guard above so the empty-ATS-registry safeguard still keys
	// off atsProviders alone. See unsignalledSources for why these are excluded.
	excluded := append(atsProviders, unsignalledSources...)

	candidates, staleCandidates, err := selectProbeCandidates(ctx, queries,
		probeScope{source: *sourceFilter, jobIDs: jobIDs}, excluded, probeDespiteRegistered, time.Now().Add(-staleCutoff))
	if err != nil {
		log.Printf("select candidates: %v", err)
		return 1
	}
	orphanCount := len(candidates)
	// Probes go out on safehttp: orphan-job URLs come from attacker-influenced sources
	// (telegram posts), so the probe must refuse internal/metadata targets.
	client := safehttp.NewClient(probeTimeout)
	paced := pacedPasses(client)
	var getProbeStale, himalayasStale, echojobsStale []candidate
	pacedStale := make(map[string][]candidate)
	for _, c := range staleCandidates {
		switch {
		case slices.Contains(probeDespiteRegisteredGET, c.Source):
			getProbeStale = append(getProbeStale, staleCandidate(c))
		case c.Source == "himalayas":
			himalayasStale = append(himalayasStale, staleCandidate(c))
		case c.Source == "echojobs":
			echojobsStale = append(echojobsStale, staleCandidate(c))
		case slices.ContainsFunc(paced, func(p pacedPass) bool { return p.source == c.Source }):
			pacedStale[c.Source] = append(pacedStale[c.Source], staleCandidate(c))
		default:
			// Unreachable in practice — every probeDespiteRegistered member is one of
			// the branches above — but a candidate silently going unverified
			// (rather than erroring loudly) is exactly the class of bug this worker
			// exists to avoid, so a future member added to probeDespiteRegistered
			// without wiring its evidence path here fails visibly instead.
			log.Printf("liveness: %s has no wired evidence path for probeDespiteRegistered — skipping %s", c.Source, c.PublicSlug)
		}
	}
	pacedCounts := make([]string, 0, len(paced))
	for _, p := range paced {
		pacedCounts = append(pacedCounts, fmt.Sprintf("%d %s", len(pacedStale[p.source]), p.source))
	}
	log.Printf("liveness: %d orphan candidates (excluding %d ATS providers + %d unsignalled sources) + %d stale candidates from %d registered sources (%d GET, %d himalayas, %d echojobs, %s)",
		orphanCount, len(atsProviders), len(unsignalledSources), len(staleCandidates), len(probeDespiteRegistered),
		len(getProbeStale), len(himalayasStale), len(echojobsStale), strings.Join(pacedCounts, ", "))
	if *sourceFilter != "" {
		log.Printf("liveness: restricted to source %q for this run", *sourceFilter)
	}

	var st stats

	if len(jobIDs) == 0 {
		// A source's stated application deadline remains authoritative even when a crawl
		// keeps listing the posting, or its board is quiet or temporarily unreachable.
		deadlineExpired, err := queries.CloseJobsPastApplicationDeadline(ctx, *sourceFilter)
		if err != nil {
			st.failed.Add(1)
			log.Printf("liveness: expire application deadlines: %v", err)
		} else {
			log.Printf("liveness: expired %d jobs past their application deadline", deadlineExpired)
		}

		// What the probe cannot judge, age judges. Runs before probing so a run that dies
		// partway still expires. A -source run on an unrelated source expires nothing here.
		sourcesToExpire := filterSources(unsignalledSources, *sourceFilter)
		expired, err := queries.CloseStaleUnsignalledJobs(ctx, db.CloseStaleUnsignalledJobsParams{
			Sources: sourcesToExpire,
			Cutoff:  pgtype.Timestamptz{Time: time.Now().Add(-expiryWindow), Valid: true},
		})
		if err != nil {
			// Log and carry on: a failing guess-half (e.g. a lock wait against a concurrent
			// reindex) must not disable the evidence-half on every later run.
			log.Printf("liveness: expire stale unsignalled jobs: %v", err)
		} else {
			log.Printf("liveness: expired %d jobs posted more than %d days ago from %d unsignalled sources",
				expired, int(expiryWindow.Hours()/24), len(sourcesToExpire))
		}

		// Age fallback for expireDespiteRegistered sources: these are re-crawled, so the
		// crawl's own reading must agree with the age guess before it closes. See unseenWindow.
		sourcesToExpireDespiteRegistered := filterSources(expireDespiteRegistered, *sourceFilter)
		unseenFor := unseenWindow(sourcesToExpireDespiteRegistered, registeredSweepGrace)
		expiredRegistered, err := queries.CloseStaleUnseenUnprobeableJobs(ctx, db.CloseStaleUnseenUnprobeableJobsParams{
			Sources:    sourcesToExpireDespiteRegistered,
			Cutoff:     pgtype.Timestamptz{Time: time.Now().Add(-expiryWindow), Valid: true},
			SeenCutoff: pgtype.Timestamptz{Time: time.Now().Add(-unseenFor), Valid: true},
		})
		if err != nil {
			log.Printf("liveness: expire stale expireDespiteRegistered jobs: %v", err)
		} else {
			log.Printf("liveness: expired %d jobs posted more than %d days ago and unseen for %d days, from %d expireDespiteRegistered sources",
				expiredRegistered, int(expiryWindow.Hours()/24), int(unseenFor.Hours()/24), len(sourcesToExpireDespiteRegistered))
		}

	} else {
		log.Printf("liveness: targeted probe of %d IDs; bulk expiry disabled", len(jobIDs))
	}

	// Each candidate set is judged by its own source's evidence, named at the call site.
	// There is deliberately no source->judge registry: an unwired source must be a visible
	// omission (the switch above logs one), not a default that probes the wrong thing.
	probe := probeJudge(client)
	st.applyVerdicts(ctx, queries, orphanCandidates(candidates), probe)
	// probeDespiteRegisteredGET candidates use the same plain-GET probe as the orphans.
	st.applyVerdicts(ctx, queries, getProbeStale, probe)
	// himalayas and echojobs have their own evidence and can't share the GET probe.
	applyHimalayasVerdicts(ctx, client, queries, himalayasStale, &st)
	st.applyVerdicts(ctx, queries, echojobsStale, echoJobsJudge(client))
	// Last, because they are slow: each paces itself against its own site (see paced.go),
	// side by side.
	st.applyPacedPasses(ctx, queries, pacedStale, paced)

	probed, closed, struck, failed := st.probed.Load(), st.closed.Load(), st.struck.Load(), st.failed.Load()
	log.Printf("liveness done: probed=%d closed=%d struck=%d failed=%d", probed, closed, struck, failed)
	return worker.ExitCode(int(failed), 0)
}

// candidate is one open job this run may judge. The two candidate queries return row types
// differing by one field (ExternalID, which echojobs needs), so the fan-out takes this.
type candidate struct {
	id         int64
	publicSlug string
	source     string
	url        string
	externalID string
	strikes    int32
}

func orphanCandidates(rows []db.SelectOrphanLivenessCandidatesRow) []candidate {
	out := make([]candidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, candidate{
			id: r.ID, publicSlug: r.PublicSlug, source: r.Source, url: r.URL, strikes: r.LivenessStrikes,
		})
	}
	return out
}

func staleCandidate(r db.SelectStaleRegisteredCandidatesRow) candidate {
	return candidate{
		id: r.ID, publicSlug: r.PublicSlug, source: r.Source, url: r.URL,
		externalID: r.ExternalID, strikes: r.LivenessStrikes,
	}
}
