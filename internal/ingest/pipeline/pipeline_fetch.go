package pipeline

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

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
func (r Runner) fetchBoard(ctx context.Context, e sources.CompanyEntry, src sources.Source) (fetchedBoard, error) {
	if r.recent() {
		rs, ok := src.(sources.RecentSource)
		if !ok {
			return fetchedBoard{}, fmt.Errorf("provider %q has no recent-only listing", e.Provider)
		}
		jobs, err := rs.FetchRecent(ctx, e, r.RecentWindow)
		return fetchedBoard{jobs: jobs}, err
	}
	hs, hydrating := src.(sources.HydratingSource)
	sl, canLookUp := r.Store.(SeenLookup)
	if !hydrating || !canLookUp {
		jobs, err := src.Fetch(ctx, e)
		return fetchedBoard{jobs: jobs}, err
	}
	set, err := sl.ExistingExternalIDs(ctx, e.Provider, e.Board)
	seenUnknown := err != nil
	if seenUnknown {
		log.Printf("ingest: %s board %q seen-set lookup failed, hydrating every posting as new and withholding its closes: %v",
			e.Provider, e.Board, err)
		set = nil
	}
	// askedNew records every posting the adapter asked about that the catalogue does not hold
	// yet. A detail-budgeted adapter skips the new ones it cannot afford without a word, so this
	// is the only way the run can tell a quiet board from one whose backlog is waiting.
	var askedMu sync.Mutex
	askedNew := map[string]bool{}
	seen := func(externalID string) bool {
		_, ok := set[externalid.Namespace(e.Board, externalID)]
		if !ok {
			askedMu.Lock()
			askedNew[externalID] = true
			askedMu.Unlock()
		}
		return ok
	}
	deferredAny := func(jobs []sources.Job) bool {
		askedMu.Lock()
		defer askedMu.Unlock()
		returned := make(map[string]bool, len(jobs))
		for _, j := range jobs {
			returned[j.ExternalID] = true
		}
		for id := range askedNew {
			if !returned[id] {
				return true
			}
		}
		return false
	}
	// An adapter that can spend its hydration budget better when it knows which postings the
	// run will discard gets the gate, but only when the coverage gate actually applies to this
	// board — otherwise its coverage answer would be a meaningless "nothing is covered".
	if cg, gated := src.(sources.CoverageGated); gated {
		if covered := r.coverageProbe(ctx, e); covered != nil {
			gate := sources.DetailGate{Covered: covered, Rejected: listingRejected(e)}
			jobs, err := cg.FetchNewGated(ctx, e, seen, gate)
			return fetchedBoard{jobs, set, deferredAny(jobs), seenUnknown}, err
		}
	}
	jobs, err := hs.FetchNew(ctx, e, seen)
	return fetchedBoard{jobs, set, deferredAny(jobs), seenUnknown}, err
}

// fetchedBoard is what fetchBoard read from one board. deferred reports that a hydrating adapter
// left a NEW posting it listed un-hydrated (its detail budget ran out, or the detail failed), so
// the board has work pending; seenUnknown that the seen-set lookup failed, so every stored
// posting read as new (see Stats.SeenUnknown).
type fetchedBoard struct {
	jobs        []sources.Job
	seen        map[string]bool
	deferred    bool
	seenUnknown bool
}

// coverageProbe builds the resolver a CoverageGated adapter consults mid-crawl: given the
// companies it has listed, which are already covered by a non-aggregator source. It is the
// same lookup aggregatorCoverageForBatch runs after the fetch, moved to where it can still
// save a request, and it is keyed by the company NAMES the adapter passed rather than by the
// slugs the lookup speaks — the adapter states what the platform told it and does not have to
// know freehire's slug rules.
//
// nil means the gate does not apply to this board, and the caller then uses the plain
// hydrating path. A failed lookup answers "nothing is covered" rather than nil: by then the
// crawl is already under way, and reporting no coverage costs a wasted request per posting
// where reporting coverage wrongly would cost a body the catalogue needed.
func (r Runner) coverageProbe(ctx context.Context, e sources.CompanyEntry) func([]string) map[string]bool {
	aggregators, countries, applies := r.aggregatorGate(e)
	if !applies {
		return nil
	}
	return func(companies []string) map[string]bool {
		// Resolve before asking, for the same reason the buffered path does: the probe is
		// deciding whether a posting is worth a detail request, and the answer has to be
		// about the employer the catalogue actually holds. In ONE batch, as the buffered path
		// does: resolving company by company cost shine (~62k employers, ~36 ms from the
		// crawl host to the database) its entire crawl budget before the first posting page.
		derived := distinctSlugs(companies)
		canon := r.resolveSlugAliases(ctx, e, derived)
		slugs := resolveAll(canon, derived)
		if len(slugs) == 0 {
			return nil
		}
		covered, err := r.Coverage.NonAggregatorCompanies(ctx, slugs, aggregators, countries)
		if err != nil {
			log.Printf("ingest: %s board %q (%s): coverage probe failed, hydrating every posting: %v",
				e.Provider, e.Board, e.Company, err)
			return nil
		}
		out := make(map[string]bool, len(covered))
		for _, c := range companies {
			if covered[canonicalOf(canon, normalize.CompanySlug(c))] {
				out[c] = true
			}
		}
		return out
	}
}

// touch refreshes an already-ingested posting's liveness (last_seen_at, reopen) by identity,
// and its stated listing deadline without rewriting hydrated content. It routes through the Store's optional Toucher capability; a
// Store without it (a test fake) drops the refresh, matching how ingestStream handles Closer.
func (r Runner) touch(ctx context.Context, e sources.CompanyEntry, j sources.Job) (string, error) {
	t, ok := r.Store.(Toucher)
	if !ok {
		return "", nil
	}
	source, externalID := jobIdentity(e, j)
	return t.Touch(ctx, source, externalID, j.ApplicationDeadline)
}

// skippedBoard is the sweep exclusion for a board the run skipped. A boardless entry has no
// namespace of its own — skipping it skips the whole provider, which then ingests nothing and
// is not swept at all — so it contributes no pattern.
func skippedBoard(e sources.CompanyEntry) []string {
	if e.Board == "" {
		return nil
	}
	return []string{externalid.BoardPattern(e.Board)}
}
