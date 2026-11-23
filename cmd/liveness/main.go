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
	"github.com/strelov1/freehire/internal/platform/pgconv"
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
)

// unsignalledSources carry no close signal at all: no re-crawl that could stop seeing
// them, no change feed, and a stored URL that is a container outliving the vacancy rather
// than its own page, so a probe can never reach a death verdict. Only telegram qualifies:
// its URL is usually the Telegram post, which stays live after the job is filled, and an
// apply link it names is mostly LinkedIn, which answers a bot with a login wall.
//
// This one list is both the probe exclusion and the age-close set, so a source can never
// be dropped from the probe without something else taking over its closes. The exception
// is a posting whose stored URL is itself a LinkedIn or Workday posting (a channel that
// relays the link): unsignalledPass routes it to that host's paced pass, and the age rule
// stays its backstop.
var unsignalledSources = []string{"telegram"}

// unsignalledPass names the paced pass that can judge an unsignalledSource posting by its
// stored URL, or "" when none can (a t.me post, an unknown host) and only age closes it.
// The plain GET probe is deliberately not used: Workday, Ashby and other SPA hosts answer
// it with a JS shell that Classify reads as expired.
func unsignalledPass(rawURL string) string {
	switch {
	case sources.LinkedInPostingID(rawURL) != "":
		return "linkedin"
	case workdayJobAPI(rawURL) != "":
		return "workday"
	}
	return ""
}

// routeUnsignalled returns the orphans the plain GET probe judges and adds every
// unsignalledSource posting unsignalledPass can judge to that pass's queue in paced, so it
// shares the host's pace and per-run budget. The rest are dropped: only age closes them.
func routeUnsignalled(rows []db.SelectOrphanLivenessCandidatesRow, paced map[string][]candidate) (orphans []db.SelectOrphanLivenessCandidatesRow, routed int) {
	for _, r := range rows {
		if !slices.Contains(unsignalledSources, r.Source) {
			orphans = append(orphans, r)
		} else if pass := unsignalledPass(r.URL); pass != "" {
			paced[pass] = append(paced[pass], orphanCandidate(r))
			routed++
		}
	}
	return orphans, routed
}

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
// past the default staleness window (staleCutoff).
//
// Per-source leaks: empregos reads only postings that name a state and cannot prove it
// read the rest; djinni lists only the last month, arbeitnow the last week; 4dayweek
// drops an employer's company from the sweep scope when it leaves the feed; torre's
// sitemap still names some closed postings; jobtech is never swept, it closes from its
// change feed, and a removal missed inside the feed's 72h window is never resent.
var probeDespiteRegistered = []string{"himalayas", "echojobs", "jobicy", "remoteok", "hh", "empregos", "djinni", "4dayweek", "torre", "arbeitnow", "jobtech", "instaffo", "remotedotcom", "thehub", "reed", "arbeitsagentur", "xing", "gradcracker", "engage", "betterteam", "linkedin", "hellowork", "dice"}

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
		linkedInPass(linkedInClient(client)),
		dicePass(client),
		workdayPass(client),
	}, catalogStatusPasses(client)...)
}

// staleCutoff uses ingest's default unseen window to decide when a posting
// becomes eligible for a probe. A source can have a longer sweep grace, as Dice does.
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
	release, locked, err := worker.TryLock(ctx, pool, worker.LivenessLockKey)
	if err != nil {
		log.Printf("liveness lock: %v", err)
		return 1
	}
	if !locked {
		log.Print("liveness: another run holds the lock — exiting")
		return 0
	}
	defer release()

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

	// unsignalledSources come back as orphans and are split off below: only those
	// unsignalledPass can judge are probed (see unsignalledSources).
	cutoff := time.Now().Add(-staleCutoff)
	selected, staleCandidates, err := selectProbeCandidates(ctx, queries,
		probeScope{source: *sourceFilter, jobIDs: jobIDs}, atsProviders, probeDespiteRegistered, cutoff)
	if err != nil {
		log.Printf("select candidates: %v", err)
		return 1
	}
	// Probes go out on safehttp: orphan-job URLs come from attacker-influenced sources
	// (telegram posts), so the probe must refuse internal/metadata targets.
	client := safehttp.NewClient(probeTimeout)
	paced := pacedPasses(client)
	var getProbeStale, himalayasStale, echojobsStale []candidate
	pacedStale := make(map[string][]candidate)
	candidates, unsignalledProbed := routeUnsignalled(selected, pacedStale)
	orphanCount := len(candidates)
	for _, row := range staleCandidates {
		c := staleCandidate(row, cutoff)
		switch {
		case slices.Contains(probeDespiteRegisteredGET, c.source):
			getProbeStale = append(getProbeStale, c)
		case c.source == "himalayas":
			himalayasStale = append(himalayasStale, c)
		case c.source == "echojobs":
			echojobsStale = append(echojobsStale, c)
		case slices.ContainsFunc(paced, func(p pacedPass) bool { return p.source == c.source }):
			pacedStale[c.source] = append(pacedStale[c.source], c)
		default:
			// Unreachable in practice — every probeDespiteRegistered member is one of
			// the branches above — but a candidate silently going unverified
			// (rather than erroring loudly) is exactly the class of bug this worker
			// exists to avoid, so a future member added to probeDespiteRegistered
			// without wiring its evidence path here fails visibly instead.
			log.Printf("liveness: %s has no wired evidence path for probeDespiteRegistered — skipping %s", c.source, c.publicSlug)
		}
	}
	pacedCounts := make([]string, 0, len(paced))
	for _, p := range paced {
		pacedCounts = append(pacedCounts, fmt.Sprintf("%d %s", len(pacedStale[p.source]), p.source))
	}
	log.Printf("liveness: %d orphan candidates (excluding %d ATS providers) + %d unsignalled postings on a judgeable host + %d stale candidates from %d registered sources (%d GET, %d himalayas, %d echojobs, %s)",
		orphanCount, len(atsProviders), unsignalledProbed, len(staleCandidates), len(probeDespiteRegistered),
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
			Cutoff:  pgconv.Instant(time.Now().Add(-expiryWindow)),
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
			Cutoff:     pgconv.Instant(time.Now().Add(-expiryWindow)),
			SeenCutoff: pgconv.Instant(time.Now().Add(-unseenFor)),
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
	seenBefore pgtype.Timestamptz
}

func orphanCandidates(rows []db.SelectOrphanLivenessCandidatesRow) []candidate {
	out := make([]candidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, orphanCandidate(r))
	}
	return out
}

func orphanCandidate(r db.SelectOrphanLivenessCandidatesRow) candidate {
	return candidate{id: r.ID, publicSlug: r.PublicSlug, source: r.Source, url: r.URL, strikes: r.LivenessStrikes}
}

func staleCandidate(r db.SelectStaleRegisteredCandidatesRow, cutoff time.Time) candidate {
	return candidate{
		id: r.ID, publicSlug: r.PublicSlug, source: r.Source, url: r.URL,
		externalID: r.ExternalID, strikes: r.LivenessStrikes, seenBefore: pgconv.Instant(cutoff),
	}
}
