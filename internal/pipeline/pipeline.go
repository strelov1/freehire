// Package pipeline is the ingest write path: it dispatches each configured board to
// its source adapter, normalizes the postings, and persists them. It is independent
// of the DB layer (via Store) and of any specific platform (via the source registry).
package pipeline

import (
	"context"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// defaultConcurrency bounds how many boards are fetched at once.
const defaultConcurrency = 8

// progressInterval is how often the run emits a heartbeat with the count of boards
// crawled so far, so a stalled board (one whose fetch hangs) is visible — the count
// stops advancing — instead of the run going silent until it finishes.
const progressInterval = 60 * time.Second

// Store persists one Job aggregate and enqueues it for enrichment when needed,
// atomically. It accepts only a job.Job — a value obtainable exclusively through
// the aggregate factory (job.New) — so the write path cannot persist a posting that
// bypassed the deterministic derivation. The pipeline is unaware of the schema
// version or the outbox — that is the Store implementation's concern.
type Store interface {
	Save(ctx context.Context, j job.Job) error
}

// FormSaver is the optional Store capability for a posting whose adapter also yielded the
// platform's application form: write the job and the form together, so a captured form
// cannot be lost to a failure between the two.
//
// It exists as a capability rather than a wider Save signature because exactly one
// provider's listing carries a form, and every other Store — including every test fake —
// would otherwise take a parameter it has nothing to do with. Same discovery rule as
// Closer and Toucher: the pipeline type-asserts, and a Store that lacks it still ingests
// the posting through the plain Save. The form is then simply not captured, which is the
// right degradation — a crawl must not fail over a field nothing yet reads.
type FormSaver interface {
	SaveWithApplyForm(ctx context.Context, j job.Job, form applyform.Form) error
}

// Closer is the optional Store capability a self-closing streaming source needs: closing a
// posting by its (source, external_id) identity when the feed reports it removed. Only the
// ingest dbStore implements it; ingestStream type-asserts for it and skips removals when a
// Store lacks it, so other Store implementations (and test fakes) are unaffected.
type Closer interface {
	Close(ctx context.Context, source, externalID string) error
}

// Toucher is the optional Store capability a HydratingSource needs: refresh a posting's liveness
// (last_seen_at, reopen if closed) by its (source, external_id) identity, WITHOUT rewriting its
// content. The pipeline uses it for a posting the adapter re-listed but did not re-fetch (its
// stored content is already current); a full upsert of the content-less listing would wipe the
// hydrated description/facets. Only the ingest dbStore implements it; the pipeline type-asserts
// for it and drops the refresh when a Store lacks it (test fakes), like Closer.
type Toucher interface {
	Touch(ctx context.Context, source, externalID string) error
}

// SeenLookup is the optional Store capability a HydratingSource needs: the external_ids already
// stored for the board about to be crawled, so the adapter fetches expensive per-posting detail
// only for postings the catalogue lacks. Only the ingest dbStore implements it; the runner
// type-asserts for it and falls back to the list-only Fetch when a Store lacks it (test fakes,
// non-DB callers), so other Store implementations are unaffected.
//
// The lookup is board-scoped because it runs once per crawled board: reading the provider's whole
// catalogue for each of its boards is affordable only for a boardless adapter, which passes an
// empty board and gets the provider-wide set.
//
// Each id maps to whether its stored row carries tech evidence (is_tech). Membership drives the
// hydration decision; the flag lets a liveness refresh face the catalogue filter on the same
// evidence a write would, which a content-less listing cannot supply.
type SeenLookup interface {
	ExistingExternalIDs(ctx context.Context, source, board string) (map[string]bool, error)
}

// BoardHealth is the optional per-board health port: it tells the Runner whether a
// board is currently cooled down (skip it) and records each crawl's outcome so a
// repeatedly-failing board backs off. A nil BoardHealth disables the feature entirely
// (the Runner behaves exactly as before), so unit tests and non-DB callers are
// unaffected — the same shape as the optional Closer.
type BoardHealth interface {
	// Cooldown reports the board's cooldown_until and whether it is set. The Runner
	// skips the board when it is set and in the future.
	Cooldown(ctx context.Context, provider, board, region string) (time.Time, bool, error)
	// RecordSuccess clears the board's failure state and stamps freshness.
	RecordSuccess(ctx context.Context, provider, board, region string, ingested int) error
	// RecordFailure counts a failed crawl and cools the board down per the backoff policy.
	RecordFailure(ctx context.Context, provider, board, region, errMsg string) error
	// CooledBoards returns up to limit (board, region) pairs of the provider currently in
	// an active cooldown — the recovery probe's candidates.
	CooledBoards(ctx context.Context, provider string, limit int) ([]CooledBoard, error)
	// ClearCooldowns clears the active cooldown of every currently-cooled board of the
	// provider and returns how many were cleared. Called once a probe proves the provider
	// reachable again.
	ClearCooldowns(ctx context.Context, provider string) (int, error)
}

// CooledBoard identifies one board currently in an active cooldown. Region disambiguates a
// board id that repeats across independent regional slices of one provider (e.g. Adzuna's
// "it-jobs" once per country) — without it, every region's health/cooldown state would
// collide on the same (provider, board) identity. A provider whose board is region-invariant
// (everyone but Adzuna today) reports Region == "".
type CooledBoard struct {
	Board  string
	Region string
}

// Stats reports what a run did: Ingested counts saved jobs, Failed counts boards that
// errored (unknown provider or a fetch failure), Skipped counts jobs that fetched fine
// but failed to persist, Cooled counts boards skipped because they are in cooldown (a
// deliberate back-off, distinct from a failure), and Rejected counts postings the
// catalogue filter turned away before the write path. Skipped is surfaced so a run whose
// every save fails (e.g. a DB schema drift) is not mistaken for a clean success.
//
// Rejected is kept apart from Skipped because the two mean opposite things: a rejection
// is the filter working as intended, a skip is something broken. Folded together, a board
// whose every save fails would read as a board full of non-technical postings.
type Stats struct {
	Ingested int
	Failed   int
	Skipped  int
	Cooled   int
	Rejected int
}

// add accumulates another Stats into s, so the per-board and per-provider merges cannot
// drift apart when a counter is added.
func (s *Stats) add(o Stats) {
	s.Ingested += o.Ingested
	s.Failed += o.Failed
	s.Skipped += o.Skipped
	s.Cooled += o.Cooled
	s.Rejected += o.Rejected
}

// RunStats is a run's outcome broken down by provider. A run may cover several providers
// (a mixed board file), and the post-run unseen-job sweep is per provider, so the breakdown
// is kept rather than a single aggregate; Total folds it back when only the sum is needed.
type RunStats map[string]Stats

// Total sums the per-provider stats into one aggregate (for the run's done-log line).
func (rs RunStats) Total() Stats {
	var t Stats
	for _, s := range rs {
		t.add(s)
	}
	return t
}

// Runner drives ingest: for each configured board it looks up the adapter, fetches,
// normalizes, and saves. Boards run concurrently up to defaultConcurrency; a board
// failure is isolated and never aborts the run.
type Runner struct {
	Registry map[string]sources.Source
	Store    Store
	// BoardHealth is optional (nil disables per-board cooldown/health recording).
	BoardHealth BoardHealth
}

// Run ingests every configured board and returns the stats per provider. It returns an
// error only for a context cancellation, never for a single board's failure. All boards
// run in one bounded concurrent pool regardless of provider, so a slow self-pacing
// provider occupies one slot without blocking the others.
func (r Runner) Run(ctx context.Context, entries []sources.CompanyEntry) (RunStats, error) {
	// Half-open the circuit breaker before the crawl: a provider whose boards were mass-
	// cooled by a since-resolved outage recovers this cycle instead of each board waiting
	// out its own backoff. The board that answered the probe was already fully ingested
	// from that same fetch (see probeProvider) — handled carries its Stats so the loop
	// below does not crawl it a second time.
	handled := r.recoverProviders(ctx, entries)

	var (
		mu     sync.Mutex
		byProv = RunStats{}
		wg     sync.WaitGroup
	)

	// Heartbeat the crawl progress: boards run concurrently and a successful board
	// logs nothing, so without this a run that stalls on one hung board is silent.
	var crawled atomic.Int64
	total := len(entries)
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("ingest: progress %d/%d boards crawled", crawled.Load(), total)
	})
	defer stopHeartbeat()
	sem := make(chan struct{}, defaultConcurrency)

	for _, e := range entries {
		wg.Add(1)
		go func(e sources.CompanyEntry) {
			defer wg.Done()

			// Acquire a slot, but abandon the board if the run is cancelled — both
			// before starting and while parked waiting for a slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}

			boardStats, already := handled[boardKey{e.Provider, e.Board, e.Region}]
			if !already {
				boardStats = r.ingestBoard(ctx, e)
			}
			crawled.Add(1)

			mu.Lock()
			s := byProv[e.Provider]
			s.add(boardStats)
			byProv[e.Provider] = s
			mu.Unlock()
		}(e)
	}
	wg.Wait()

	return byProv, ctx.Err()
}

// maxRecoveryProbes bounds how many cooled boards a provider is probed with before a
// run: more than one so a single genuinely-dead board among the cooled set does not mask
// a recovered provider, few enough that the probe stays cheap.
const maxRecoveryProbes = 3

// boardKey identifies one board within a run: the pair a CompanyEntry, a Claimed
// cooldown, and a recovery probe all key on.
type boardKey struct{ provider, board, region string }

// recoverProviders is the provider-level circuit breaker's half-open transition. Before
// the main crawl, for each provider with cooled boards it probes up to maxRecoveryProbes
// of them through the adapter; the first probe that succeeds proves the provider is
// reachable again, so it clears the provider's cooldowns and the run crawls the rest this
// cycle rather than leaving each board to ride out its per-board backoff (up to a day)
// after a resolved provider-wide outage. Every probe failing leaves the provider cooled,
// so a still-down provider is never stampeded. A nil BoardHealth port disables it entirely.
//
// It returns the Stats of any board the probe itself fully ingested (see probeProvider):
// for such a board the caller must not run ingestBoard again this cycle, or "a few cheap
// probes" becomes a full second crawl of the one board that happened to answer.
func (r Runner) recoverProviders(ctx context.Context, entries []sources.CompanyEntry) map[boardKey]Stats {
	handled := make(map[boardKey]Stats)
	if r.BoardHealth == nil {
		return handled
	}
	byProvider := entriesByProvider(entries)
	for _, provider := range sortedProviders(byProvider) {
		src, ok := r.Registry[provider]
		if !ok {
			continue
		}
		// The probe only pays off with leverage — a few cheap probes clearing many boards.
		// A lone board has none: probing it crawls the whole provider (for a boardless
		// aggregator, its entire dataset) only for the main loop to crawl it again. Skip it;
		// the normal cooldown gate recovers a single board in one crawl at cooldown expiry.
		if len(byProvider[provider]) < 2 {
			continue
		}
		boards, err := r.BoardHealth.CooledBoards(ctx, provider, maxRecoveryProbes)
		if err != nil {
			log.Printf("ingest: recovery probe %s: list cooled boards: %v", provider, err)
			continue
		}
		e, st, reused, answered := r.probeProvider(ctx, src, boards, byProvider[provider])
		if !answered {
			continue
		}
		cleared, err := r.BoardHealth.ClearCooldowns(ctx, provider)
		if err != nil {
			log.Printf("ingest: recovery probe %s: clear cooldowns: %v", provider, err)
			continue
		}
		log.Printf("ingest: %s answered a recovery probe — cleared %d cooled board(s) to crawl this cycle", provider, cleared)
		if reused {
			handled[boardKey{e.Provider, e.Board, e.Region}] = st
		}
	}
	return handled
}

// probeProvider fetches each candidate board through the adapter until one succeeds,
// reporting the entry that answered. For a non-streaming board — the common case, and
// the only one fetchBoard's result actually corresponds to — that same fetch is finished
// into a full ingest (reused=true, st holds its Stats) instead of being thrown away, so
// the board is not crawled a second time by the main loop; the finding this fixes was a
// HydratingSource's FetchNew (a full paginated re-crawl plus per-posting hydration, not a
// cheap reachability check) run once to answer the probe and once more moments later. A
// streaming adapter's real ingest path is ingestStream, which this fetch does not
// exercise, so reused is false for it and the main loop still runs it normally. A board
// absent from this run's entries is skipped (it cannot be probed without its entry), and
// a cancelled run stops early. answered=false when nothing responded.
func (r Runner) probeProvider(ctx context.Context, src sources.Source, boards []CooledBoard, entries map[entryKey]sources.CompanyEntry) (e sources.CompanyEntry, st Stats, reused, answered bool) {
	_, streaming := src.(sources.StreamingSource)
	for _, board := range boards {
		if ctx.Err() != nil {
			return sources.CompanyEntry{}, Stats{}, false, false
		}
		candidate, ok := entries[entryKey{board.Board, board.Region}]
		if !ok {
			continue
		}
		raw, seen, err := r.fetchBoard(ctx, candidate, src)
		if err != nil {
			continue
		}
		if streaming {
			return candidate, Stats{}, false, true
		}
		return candidate, r.ingestFetched(ctx, candidate, raw, seen), true, true
	}
	return sources.CompanyEntry{}, Stats{}, false, false
}

// entryKey is a board's identity within one provider: board alone collides when a provider's
// board id repeats across independent regional slices (e.g. Adzuna's "it-jobs" once per
// country), so region joins it. A region-invariant provider's entries all carry Region == "".
type entryKey struct{ board, region string }

// entriesByProvider indexes the run's entries as provider → entryKey → entry, so the
// recovery probe can find the CompanyEntry for a cooled (board, region) pair.
func entriesByProvider(entries []sources.CompanyEntry) map[string]map[entryKey]sources.CompanyEntry {
	byProvider := make(map[string]map[entryKey]sources.CompanyEntry)
	for _, e := range entries {
		boards := byProvider[e.Provider]
		if boards == nil {
			boards = make(map[entryKey]sources.CompanyEntry)
			byProvider[e.Provider] = boards
		}
		boards[entryKey{e.Board, e.Region}] = e
	}
	return byProvider
}

// sortedProviders returns the run's providers deterministically, so recovery probes (and
// tests) are order-stable.
func sortedProviders(byProvider map[string]map[entryKey]sources.CompanyEntry) []string {
	providers := make([]string, 0, len(byProvider))
	for p := range byProvider {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return providers
}

// ingestBoard fetches and saves one board, returning that board's contribution to the
// run stats: Ingested counts saved jobs, Failed is 1 when the board itself errored,
// Skipped counts jobs that fetched fine but failed to persist, Rejected counts postings
// the catalogue filter turned away, and Cooled is 1 when the board was skipped for
// cooldown. A missing adapter or a fetch error fails the board; a per-job save error
// skips that job without failing the board, but is counted and logged so it is never
// silently swallowed. A board in cooldown is skipped before its adapter is touched, and
// each crawl's board-level outcome is recorded to BoardHealth.
func (r Runner) ingestBoard(ctx context.Context, e sources.CompanyEntry) Stats {
	// Cooldown gate — before the adapter lookup, so a backed-off board costs nothing.
	if r.cooledDown(ctx, e) {
		log.Printf("ingest: %s board %q (%s) in cooldown — skipping", e.Provider, e.Board, e.Company)
		return Stats{Cooled: 1}
	}

	src, ok := r.Registry[e.Provider]
	if !ok {
		log.Printf("ingest: %s/%s: unknown provider %q", e.Company, e.Board, e.Provider)
		r.recordFailure(ctx, e, "unknown provider "+e.Provider)
		return Stats{Failed: 1}
	}

	// A streaming adapter persists postings as it crawls, so a long rate-limited board's
	// progress is saved incrementally (and survives an interrupted run) rather than buffered
	// until the whole board finishes.
	if ss, streaming := src.(sources.StreamingSource); streaming {
		st := r.ingestStream(ctx, e, ss)
		// A streaming board that made no progress at all AND failed is a true outage;
		// partial progress is a success signal, not a board failure. A rejected posting
		// is progress: the crawl reached it and the filter turned it away. Without that,
		// a mid-crawl error on an all-rejected board — the normal case on the non-tech-heavy
		// national feeds — would count as a failure and cool a healthy board for hours.
		if st.Failed > 0 && st.Ingested == 0 && st.Rejected == 0 {
			r.recordFailure(ctx, e, "streaming board failed with no progress")
		} else {
			r.recordSuccess(ctx, e, st.Ingested)
		}
		return st
	}

	raw, seen, err := r.fetchBoard(ctx, e, src)
	if err != nil {
		// Log the cause so a failed board is diagnosable (the source error carries
		// the HTTP status / timeout); the run still isolates and continues.
		log.Printf("ingest: %s board %q (%s) failed: %v", e.Provider, e.Board, e.Company, err)
		r.recordFailure(ctx, e, err.Error())
		return Stats{Failed: 1}
	}
	return r.ingestFetched(ctx, e, raw, seen)
}

// ingestFetched turns one board's already-fetched raw postings into Stats: applying a
// HydratingSource's liveness refreshes, filtering and saving the rest through the
// catalogue, and recording the board's health. Split out of ingestBoard so the recovery
// probe — which must fetch a board through the adapter to answer it — can finish that
// same fetch into a full ingest (see probeProvider) instead of discarding the result and
// having the main loop fetch the board a second time.
func (r Runner) ingestFetched(ctx context.Context, e sources.CompanyEntry, raw []sources.Job, seen map[string]bool) Stats {
	var (
		st       Stats
		rej      rejections
		firstErr error
	)
	for _, j := range raw {
		// A HydratingSource marks an already-ingested posting it re-listed but did not
		// re-fetch: refresh its liveness by identity instead of re-upserting content-less
		// (which would wipe the description/facets hydrated when it was new).
		if j.SeenRefresh {
			// A refresh faces the same catalogue filter as a write, so a posting the
			// dictionary turns away ages out of the catalogue instead of being kept alive
			// by its own re-listing. Judged on the STORED tech evidence, not on the
			// content-less listing: the dictionary matches its terms anywhere in a title,
			// and a title alone would condemn engineering roles whose evidence lives in
			// the description they were hydrated from.
			rej.candidate()
			_, externalID := jobIdentity(e, j)
			if outOfCatalogueTitle(j.Title, seen[externalID]) {
				rej.reject(j.Title)
				continue
			}
			if err := r.touch(ctx, e, j); err != nil {
				st.Skipped++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			st.Ingested++
			continue
		}
		r.saveOne(ctx, e, j, &st, &rej, &firstErr)
	}
	// One line per board with skips (not one per job), so a systemic failure — e.g.
	// the DB behind a migration, or a board whose postings won't construct — is visible
	// without flooding the log.
	if st.Skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on construct or save error (e.g. %v)",
			e.Provider, e.Board, e.Company, st.Skipped, len(raw), firstErr)
	}
	rej.log(e)
	// The board was reachable (Fetch succeeded), so it is healthy regardless of per-job
	// save skips — those are stats.Skipped, not a board outage.
	r.recordSuccess(ctx, e, st.Ingested)
	st.Rejected = rej.rejected
	return st
}

// saveOne normalizes one posting, puts it through the catalogue filter, and saves what
// survives, tallying the outcome into st and keeping the first error for the board's
// skip-summary log line. Both the buffered (ingestBoard) and the streaming (ingestStream)
// save loops share it so the normalize→filter→save→count step behaves identically; the
// SeenRefresh and Removed branches stay on the caller, which is why neither reaches the
// filter's candidate denominator.
func (r Runner) saveOne(ctx context.Context, e sources.CompanyEntry, j sources.Job, st *Stats, rej *rejections, firstErr *error) {
	dj, err := normalizeJob(e, j)
	if err != nil {
		st.Skipped++
		if *firstErr == nil {
			*firstErr = err
		}
		return
	}
	rej.candidate()
	if outOfCatalogue(dj) {
		rej.reject(dj.Fields().Title)
		return
	}
	if err := r.save(ctx, dj, j.ApplyForm); err != nil {
		st.Skipped++
		if *firstErr == nil {
			*firstErr = err
		}
		return
	}
	st.Ingested++
}

// save writes the posting, taking the form along when the adapter yielded one and the
// Store can hold it. Both conditions matter: a nil form is the normal case for almost
// every provider, and a Store without the capability must still ingest.
func (r Runner) save(ctx context.Context, dj job.Job, form *applyform.Form) error {
	if form != nil {
		if fs, ok := r.Store.(FormSaver); ok {
			return fs.SaveWithApplyForm(ctx, dj, *form)
		}
	}
	return r.Store.Save(ctx, dj)
}

// fetchBoard fetches a board's postings, preferring a hydrating adapter's FetchNew — which
// fetches per-posting detail (e.g. the description the list omits) only for postings not already
// ingested — when the adapter opts in AND the Store can supply the board's seen-set. It falls
// back to the list-only Fetch otherwise. The seen predicate namespaces the adapter's raw posting
// id the same way the write path does, so it matches the stored external_id. A seen-set lookup
// error fails OPEN (empty set → every posting treated as new), so a health hiccup never skips the
// board.
//
// It returns the seen-set alongside the postings: it maps each stored external_id to that row's
// tech evidence, which the caller needs to judge a liveness refresh against the same rule as a
// write. A non-hydrating board has no seen-set and returns nil, which reads as "no evidence" for
// every lookup — harmless, because such a board yields no refreshes.
func (r Runner) fetchBoard(ctx context.Context, e sources.CompanyEntry, src sources.Source) ([]sources.Job, map[string]bool, error) {
	hs, hydrating := src.(sources.HydratingSource)
	sl, canLookUp := r.Store.(SeenLookup)
	if !hydrating || !canLookUp {
		jobs, err := src.Fetch(ctx, e)
		return jobs, nil, err
	}
	set, err := sl.ExistingExternalIDs(ctx, e.Provider, e.Board)
	if err != nil {
		log.Printf("ingest: %s board %q seen-set lookup failed, hydrating every posting as new: %v",
			e.Provider, e.Board, err)
		set = nil
	}
	seen := func(externalID string) bool {
		_, ok := set[sources.NamespaceExternalID(e.Board, externalID)]
		return ok
	}
	jobs, err := hs.FetchNew(ctx, e, seen)
	return jobs, set, err
}

// touch refreshes an already-ingested posting's liveness (last_seen_at, reopen) by identity,
// without rewriting its content. It routes through the Store's optional Toucher capability; a
// Store without it (a test fake) drops the refresh, matching how ingestStream handles Closer.
func (r Runner) touch(ctx context.Context, e sources.CompanyEntry, j sources.Job) error {
	t, ok := r.Store.(Toucher)
	if !ok {
		return nil
	}
	source, externalID := jobIdentity(e, j)
	return t.Touch(ctx, source, externalID)
}

// cooledDown reports whether a board is currently backed off. It fails OPEN: a health
// lookup error is logged and the board is crawled anyway, so a health-store hiccup never
// stalls ingest. Always false when no BoardHealth port is wired.
func (r Runner) cooledDown(ctx context.Context, e sources.CompanyEntry) bool {
	if r.BoardHealth == nil {
		return false
	}
	until, set, err := r.BoardHealth.Cooldown(ctx, e.Provider, e.Board, e.Region)
	if err != nil {
		log.Printf("ingest: cooldown check %s/%s: %v", e.Provider, e.Board, err)
		return false
	}
	return set && until.After(time.Now())
}

// recordSuccess / recordFailure persist a board's outcome best-effort: a health-store
// error is logged and never propagated (health must not fail ingest), and both no-op
// when no BoardHealth port is wired.
func (r Runner) recordSuccess(ctx context.Context, e sources.CompanyEntry, ingested int) {
	if r.BoardHealth == nil {
		return
	}
	if err := r.BoardHealth.RecordSuccess(ctx, e.Provider, e.Board, e.Region, ingested); err != nil {
		log.Printf("ingest: record board success %s/%s: %v", e.Provider, e.Board, err)
	}
}

func (r Runner) recordFailure(ctx context.Context, e sources.CompanyEntry, msg string) {
	if r.BoardHealth == nil {
		return
	}
	if err := r.BoardHealth.RecordFailure(ctx, e.Provider, e.Board, e.Region, msg); err != nil {
		log.Printf("ingest: record board failure %s/%s: %v", e.Provider, e.Board, err)
	}
}

// ingestStream drives a streaming board: it persists each posting the adapter emits as it is
// crawled, so progress is durable mid-run. The emit sink normalizes and saves under a mutex
// (the adapter may emit concurrently) and tallies the same ingested/skipped counts as the
// buffered path; a board-level FetchStream error counts the board failed but keeps whatever was
// already saved (the 48h unseen-sweep guards against a short crawl closing the un-reached tail).
func (r Runner) ingestStream(ctx context.Context, e sources.CompanyEntry, ss sources.StreamingSource) Stats {
	var (
		mu       sync.Mutex
		st       Stats
		rej      rejections
		firstErr error
		total    int
	)
	emit := func(j sources.Job) {
		mu.Lock()
		defer mu.Unlock()
		total++
		// A self-closing source emits removed postings: close by identity instead of
		// upserting. The Store must implement Closer (the ingest dbStore does); a Store
		// without it simply drops the removal (e.g. a test fake that never sees them).
		if j.Removed {
			c, ok := r.Store.(Closer)
			if !ok {
				return
			}
			source, externalID := jobIdentity(e, j)
			if err := c.Close(ctx, source, externalID); err != nil {
				st.Skipped++
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			// A close is not counted as ingested — Stats.Ingested is saved jobs only.
			return
		}
		r.saveOne(ctx, e, j, &st, &rej, &firstErr)
	}

	err := ss.FetchStream(ctx, e, emit)
	rej.log(e)
	if st.Skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on save error (e.g. %v)",
			e.Provider, e.Board, e.Company, st.Skipped, total, firstErr)
	}
	if err != nil {
		log.Printf("ingest: %s board %q (%s) failed after %d saved: %v",
			e.Provider, e.Board, e.Company, st.Ingested, err)
		st.Failed = 1
		st.Rejected = rej.rejected
		return st
	}
	st.Rejected = rej.rejected
	return st
}

// jobIdentity is the dedup key a posting persists under: the provider is the source, and the
// external id is namespaced by board so two companies on one platform cannot collide. Both
// the upsert (via normalizeJob) and the stream-driven close derive identity here, so a
// removal closes exactly the row a live emit would have upserted.
func jobIdentity(e sources.CompanyEntry, j sources.Job) (source, externalID string) {
	return e.Provider, sources.NamespaceExternalID(e.Board, j.ExternalID)
}

// normalizeJob turns a raw posting into the Job aggregate through the factory: the
// platform becomes the source and the external id is namespaced by board (so two
// companies on one platform cannot collide), and job.New derives the slugs and the
// dictionary facets internally — so ingest, the moderator write path, and Telegram
// extraction all produce identical facets from the one door. It returns
// job.ErrInvalidDraft for a posting with no title/identity, which the caller skips.
func normalizeJob(e sources.CompanyEntry, j sources.Job) (job.Job, error) {
	source, externalID := jobIdentity(e, j)
	return job.New(job.Draft{
		Input: jobderive.Input{
			Source:             source,
			ExternalID:         externalID,
			Title:              j.Title,
			Company:            j.Company,
			Location:           j.Location,
			Description:        j.Description,
			WorkMode:           j.WorkMode,
			Countries:          j.Countries,
			Seniority:          j.Seniority,
			Category:           j.Category,
			EmploymentType:     j.EmploymentType,
			Skills:             j.Skills,
			ExperienceYearsMin: j.ExperienceYearsMin,
			SalaryMin:          j.SalaryMin,
			SalaryMax:          j.SalaryMax,
			SalaryCurrency:     j.SalaryCurrency,
			SalaryPeriod:       j.SalaryPeriod,
		},
		URL:      j.URL,
		Remote:   j.Remote,
		PostedAt: j.PostedAt,
	})
}
