package pipeline

import (
	"context"
	"fmt"
	"log"
	"sync"

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
	set, err := sl.ExistingExternalIDs(ctx, e.Provider, seenScope(e))
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
		_, ok := set[seenKey(e, externalID)]
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
	if cg, ok := src.(sources.CoverageGated); ok {
		gate := sources.DetailGate{Covered: func([]string) map[string]bool { return nil }}
		jobs, err := cg.FetchNewGated(ctx, e, seen, gate)
		return fetchedBoard{jobs: jobs, deferred: deferredAny(jobs), seenUnknown: seenUnknown}, err
	}
	jobs, err := hs.FetchNew(ctx, e, seen)
	return fetchedBoard{jobs: jobs, deferred: deferredAny(jobs), seenUnknown: seenUnknown}, err
}

// seenScope is the board argument of the seen-set lookup: the namespace the provider's rows are
// stored under. LinkedIn rows are keyed by the global posting id, not board:id (jobIdentity), so
// no board pattern matches them and the lookup is provider-wide.
func seenScope(e sources.CompanyEntry) string {
	if e.Provider == "linkedin" {
		return ""
	}
	return e.Board
}

// seenKey is the stored external_id an adapter's raw posting id maps to, derived from jobIdentity
// so the seen-set is keyed exactly as the write path stores. An id with no valid identity maps to
// "" and so reads as unseen.
func seenKey(e sources.CompanyEntry, externalID string) string {
	_, id := jobIdentity(e, sources.Job{ExternalID: externalID, URL: sources.LinkedInPostingURL(externalID)})
	return id
}

// fetchedBoard is what fetchBoard read from one board. deferred reports that a hydrating adapter
// left a NEW posting it listed un-hydrated (its detail budget ran out, or the detail failed), so
// the board has work pending; seenUnknown that the seen-set lookup failed, so every stored
// posting read as new (see Stats.SeenUnknown).
type fetchedBoard struct {
	jobs        []sources.Job
	deferred    bool
	seenUnknown bool
	// cut reports that the fetch's context ended under it (Runner.WriteReserve).
	cut bool
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
