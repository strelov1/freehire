package pipeline

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// ingestBoard fetches and saves one board, returning that board's contribution to the
// run stats: Ingested counts saved jobs, Failed is 1 when the board itself errored,
// Skipped counts jobs that fetched fine but failed to persist, Rejected counts postings
// the catalogue filter turned away, and Cooled is 1 when the board was skipped for
// cooldown. A missing adapter or a fetch error fails the board; a per-job save error
// skips that job without failing the board, but is counted and logged so it is never
// silently swallowed. A board in cooldown is skipped before its adapter is touched, and
// each crawl's board-level outcome is recorded to BoardHealth.
func (r Runner) ingestBoard(ctx context.Context, e sources.CompanyEntry) Stats {
	// Pause gate (failure cooldown or quiet pause) — before the adapter lookup, so a backed-off
	// board costs nothing. A quiet skip is not logged per board: it is the common case once the
	// backoff settles, and the run's summary line counts them.
	switch r.paused(ctx, e) {
	case pausedCooldown:
		log.Printf("ingest: %s board %q (%s) in cooldown — skipping", e.Provider, e.Board, e.Company)
		return Stats{Cooled: 1, SkippedBoards: skippedBoard(e)}
	case pausedQuiet:
		return Stats{Quiet: 1, SkippedBoards: skippedBoard(e)}
	}

	src, ok := r.Registry[e.Provider]
	if !ok {
		log.Printf("ingest: %s/%s: unknown provider %q", e.Company, e.Board, e.Provider)
		r.recordFailure(ctx, e, errors.New("unknown provider "+e.Provider))
		return Stats{Failed: 1}
	}

	if r.recent() {
		if _, ok := src.(sources.RecentSource); !ok {
			return Stats{RecentUnsupported: 1, SkippedBoards: skippedBoard(e)}
		}
	}

	// Independent of which Fetch variant this board takes below: a company-level
	// description is a fact about the board, not about any one posting.
	r.fillCompanyDescription(ctx, e, src)

	// A streaming adapter persists postings as it crawls, so a long rate-limited board's
	// progress is saved incrementally (and survives an interrupted run) rather than buffered
	// until the whole board finishes.
	if ss, streaming := src.(sources.StreamingSource); streaming && !r.recent() {
		st, listed := r.ingestStream(ctx, e, ss)
		// A streaming board that made no progress at all AND failed is a true outage;
		// partial progress is a success signal, not a board failure. A rejected posting is
		// progress: the crawl reached it and the filter turned it away. Without that, a
		// mid-crawl error on an all-rejected board — the normal case on the non-tech-heavy
		// national feeds — would count as a failure and cool a healthy board for hours. An
		// ATSCovered posting is progress for the same reason: the crawl reached it and
		// correctly judged its company already covered elsewhere — and omitting it here
		// would make a heavily-covered aggregator board MORE likely to be misclassified as
		// failed as the coverage gate does its job better, not less.
		if st.Failed > 0 && st.Ingested == 0 && st.Rejected == 0 && st.ATSCovered == 0 {
			r.recordFailure(ctx, e, errors.New("streaming board failed with no progress"))
		} else {
			r.recordSuccess(ctx, e, st.Ingested, boardListedAnyPosting(st), r.completeListingFingerprint(ctx, src, listed, st))
		}
		return st
	}

	fetched, err := r.fetchBoard(ctx, e, src)
	if err != nil && (ctx.Err() != nil || limiterOutOfRun(err)) {
		// The run's context ended under the fetch, or a rate limiter refused the request
		// because it could not be sent before that end: running out of time is not the board's
		// failure, the same reading markInterrupted gives a board that returned in time.
		log.Printf("ingest: %s board %q (%s) interrupted by the run's end: %v", e.Provider, e.Board, e.Company, err)
		return Stats{Interrupted: 1}
	}
	if err != nil {
		// Log the cause so a failed board is diagnosable (the source error carries
		// the HTTP status / timeout); the run still isolates and continues.
		log.Printf("ingest: %s board %q (%s) failed: %v", e.Provider, e.Board, e.Company, err)
		r.recordFailure(ctx, e, err)
		return Stats{Failed: 1}
	}
	return r.ingestFetched(ctx, e, fetched)
}

// fillCompanyDescription calls a CompanyDescriber adapter at most once per board, and only
// when the Store says the company is due (see CompanyDescriptionFiller), then records the
// answer via the Store's optional CompanyDescriptionFiller capability. Neither a missing
// capability on either side nor an error from the due-check or the adapter propagates — a
// company-description fetch must never affect the board's own job ingest, only ever add a
// company_info gap-fill on top of it. A failed due-check skips the fetch: the field is
// optional, and a database hiccup must not become a request per board against a
// rate-limited platform. A failed fetch records nothing, so it is retried next run.
func (r Runner) fillCompanyDescription(ctx context.Context, e sources.CompanyEntry, src sources.Source) {
	cd, ok := src.(sources.CompanyDescriber)
	if !ok {
		return
	}
	filler, ok := r.Store.(CompanyDescriptionFiller)
	if !ok {
		return
	}
	// Through canonicalOf, like the posting upsert: a board whose company a merge retired must
	// not write (or insert a 0-job row) under the retired slug, which would shadow the alias
	// redirect and leave the canonical company without the description.
	slug := canonicalOf(r.streamCompanyCanonical(ctx, e), normalize.CompanySlug(e.Company))
	due, err := filler.CompanyDescriptionDue(ctx, slug, time.Now().Add(-CompanyDescriptionRefreshWindow))
	if err != nil {
		log.Printf("ingest: %s board %q (%s): company description due-check: %v", e.Provider, e.Board, e.Company, err)
		return
	}
	if !due {
		return
	}
	description, err := cd.CompanyDescription(ctx, e)
	if err != nil {
		log.Printf("ingest: %s board %q (%s): company description: %v", e.Provider, e.Board, e.Company, err)
		return
	}
	if err := filler.RecordCompanyDescription(ctx, slug, e.Company, description); err != nil {
		log.Printf("ingest: %s board %q (%s): record company description: %v", e.Provider, e.Board, e.Company, err)
	}
}

// ingestFetched turns one board's already-fetched raw postings into Stats: applying a
// HydratingSource's liveness refreshes, filtering and saving the rest through the
// catalogue, and recording the board's health. Split out of ingestBoard so the recovery
// probe — which must fetch a board through the adapter to answer it — can finish that
// same fetch into a full ingest (see probeProvider) instead of discarding the result and
// having the main loop fetch the board a second time.
//
// A deferred board (fetchedBoard.deferred) has work pending, so its crawl does not count as
// quiet; one crawled without its seen-set (fetchedBoard.seenUnknown) is no evidence of what is
// gone, so its close scopes are withheld.
func (r Runner) ingestFetched(ctx context.Context, e sources.CompanyEntry, fetched fetchedBoard) Stats {
	raw, seen := fetched.jobs, fetched.seen
	canon := r.resolveCompanyAliases(ctx, e, raw)
	covered := r.aggregatorCoverageForBatch(ctx, e, raw, canon)
	t := r.writeBoard(ctx, e, raw, seen, covered, canon)
	st, rej, firstErr := t.st, t.rej, t.firstErr
	var refreshedCompanies []string
	st.Refreshed, st.RefreshFailed, refreshedCompanies = r.refreshCovered(ctx, e, t.refreshIDs)
	closed, closeFailed, closedCompanies := r.closeCovered(ctx, e, t.coveredIDs)
	st.Refreshed, st.RefreshFailed = st.Refreshed+closed, st.RefreshFailed+closeFailed
	refreshedCompanies = append(refreshedCompanies, closedCompanies...)
	// One line per board with skips (not one per job), so a systemic failure — e.g.
	// the DB behind a migration, or a board whose postings won't construct — is visible
	// without flooding the log.
	if st.Skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on construct or save error (e.g. %v)",
			e.Provider, e.Board, e.Company, st.Skipped, len(raw), firstErr)
	}
	if st.ATSCovered > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d postings — company already covered by a non-aggregator source",
			e.Provider, e.Board, e.Company, st.ATSCovered, len(raw))
	}
	rej.log(e)
	st.Rejected = rej.rejected
	// The board was reachable (Fetch succeeded), so it is healthy regardless of per-job
	// save skips — those are stats.Skipped, not a board outage.
	//
	// Deliberately AFTER st.Rejected is filled in above, not before: boardReachedPostings
	// counts a rejected posting as a posting the crawl reached, and reading st a line earlier
	// would see Rejected == 0 and report an all-rejected board as having yielded nothing —
	// which on a national feed heavy with non-tech postings is the ordinary outcome, not an
	// empty feed. The whole board would then age into the empty-feed net and have its jobs
	// closed under a diagnosis that was never true of it.
	listed := make([]string, len(raw))
	for i, j := range raw {
		listed[i] = j.ExternalID
	}
	fingerprint := ""
	if !fetched.deferred {
		fingerprint = r.completeListingFingerprint(ctx, r.Registry[e.Provider], listed, st)
	}
	r.recordSuccess(ctx, e, st.Ingested, boardListedAnyPosting(st), fingerprint)
	// Too much of the board went unread for its crawl to be evidence of what is still on it,
	// so this run's close scopes are withheld for every employer the board carries — the board
	// scope through boardQualifies, the company scope through these slugs. Announced because
	// an operator otherwise sees only a sweep that closed nothing.
	if fetched.seenUnknown {
		st.SeenUnknown = 1
		st.UnprovenCompanies = resolveAll(canon, distinctCompanySlugs(raw))
		// raw names only the companies of what the adapter yielded; a company found only among
		// the postings it left out could still be closed through another board's crawl, so the
		// board itself is excluded too.
		st.SkippedBoards = append(st.SkippedBoards, skippedBoard(e)...)
	} else if !boardReadWhatItListed(st) {
		st.UnprovenCompanies = resolveAll(canon, distinctCompanySlugs(raw))
		log.Printf("ingest: %s board %q (%s): %d/%d postings unreadable — withholding this run's stale-job close for it",
			e.Provider, e.Board, e.Company, st.Unreadable, len(raw))
	}
	if fetched.seenUnknown || !boardReadWhatItListed(st) {
		st.UnprovenCompanies = append(st.UnprovenCompanies, refreshedCompanies...)
		st.UnprovenCompanies = slices.Compact(slices.Sorted(slices.Values(st.UnprovenCompanies)))
	}
	markInterrupted(ctx, &st)
	return st
}

// markInterrupted flags a board whose crawl the run's context ended under. It is read when the
// board FINISHES, which is the only moment that answers the question: a context that is still
// live then was live for every fetch and every write before it, while one that has ended may
// have cut either short — a write refused with a cancelled context leaves that posting's
// last_seen_at where it was, and the board scope would read it as gone. A board that happened
// to finish a moment before the deadline is flagged too; that costs one crawl cycle of latency
// on its removals and nothing else.
func markInterrupted(ctx context.Context, st *Stats) {
	if ctx.Err() != nil {
		st.Interrupted = 1
	}
}

// saveConcurrency bounds how many of one board's postings are written at once. The database
// sits on another host, ~26 ms away (measured 2026-10-22), and a posting costs a round trip
// or several, so a board written one posting at a time is bounded by the link rather than by
// the database: energyjobline's ~54k liveness refreshes needed ~2h against a 50-minute unit,
// and a run that overruns stores nothing it fetched. Measured from the crawl host, 8
// connections committed 199 refreshes a second against 25 for one. Each run's pool still
// caps the total, so a multi-board provider cannot multiply this past it.
const saveConcurrency = 8
