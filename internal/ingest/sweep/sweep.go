package sweep

import (
	"cmp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// StaleAfter is the DEFAULT grace window before an unseen job is closed. An adapter declares
// its own window instead — wider for one that crawls only a slice of its catalogue, narrower
// for one whose full-board crawl makes an unseen reading positive evidence — see sweepWindowFor.
// Shared with cmd/liveness's staleCutoff via sources.DefaultSweepGrace, since liveness's
// probeDespiteRegistered backstop only picks up what this sweep has already had a chance
// to close and must not drift out of sync with it.
const StaleAfter = sources.DefaultSweepGrace

// BoardScope is the per-provider registry facts SweepableBoards gates on, resolved by the caller
// — the same convention SweepBySource uses, rather than re-deriving them from the raw registry
// maps.
type BoardScope struct {
	HasGrace, FullCatalog, FullBoardListing, SliceBoardListing, EmptyBoardListing, RegionScoped bool
}

// SweepableBoards returns, sorted and de-duplicated, the boards of one provider the
// board-scoped close may retire this run: those the run structurally proved it covered
// (stats.QualifyingBoards, see pipeline.boardQualifies), gated on the provider's markers.
//
// fullBoardListing must be true: a provider whose adapter is not registered as listing a
// board to completion never contributes to the board scope, however its crawl went — see
// sources.fullBoardListing for the bar an adapter must clear to earn it. hasGrace and
// fullCatalog exclude a provider even when registered, for the same reasons SweepBySource
// excludes them from the source-scoped close: a sweepGrace provider's crawl deliberately
// reaches only a slice of the catalogue, so a board-scoped close on the default window would
// close postings that merely drifted past the crawl's depth; a fullCatalog provider already
// closes by source alone on a clean run, strictly broader than board scope (and today's
// fullCatalog adapters are boardless besides, so this exclusion is belt-and-braces). This
// board-level decision is separate from ShouldSweep's company/source gate.
//
// sliceBoardListing is the other way in, and it requires hasGrace instead of excluding it: such
// an adapter (whatjobs) reads a board only to a page budget, so it never proves a full listing,
// and the grace window is what stands in for the proof — the board-scoped close then runs on
// that window, not the default, so a posting that merely sat past the budget is back before it
// ends. Without it a whatjobs posting closed only through the company scope, which leaked every
// posting whose company had drifted out of all the keyword slices (sources.sliceBoardListing).
//
// crossShardAmbiguous excludes a board name this run's own Stats saw as unambiguous but that
// is region-ambiguous across the FULL, unsharded catalog (pipeline.AmbiguousBoardNames,
// computed by the caller before sharding): sources.Config.Shard groups boards by company
// slug, not by board name, so a board's two region-variant rows can land in separate shard
// processes — each running its own Runner.Run and seeing only one region, concluding on its
// own that the board is unambiguous. Refusing here is what catches what a single Run() call
// structurally cannot.
//
// emptyBoardListing adds the boards the run finished whose listing named nothing
// (stats.EmptyBoards, see pipeline.boardListedEmpty) — an employer that took every posting down.
// boardQualifies refuses them, because on most adapters an empty answer cannot be told from a
// broken crawl, and their last postings then waited 30 days for the empty-feed net; an adapter
// carrying sources.emptyBoardListing has proven that it can tell them apart. Only on a complete
// listing: a slice that came back empty says nothing about the rest of the board.
//
// regionScoped keeps each board's region, for an adapter whose board ids repeat across regions
// and whose close narrows each to its region by posting URL (sources.RegionScopedBoards). Every
// other provider's region is cleared, so its board scope stays keyed on the board id alone.
//
// De-duplication guards against a board legitimately appearing twice in one run (a repeated
// board-file entry, or one board id recurring across independent regional slices) double-
// counting the close and its log line.
func SweepableBoards(stats pipeline.Stats, scope BoardScope, crossShardAmbiguous map[string]bool) []pipeline.BoardRef {
	complete := scope.FullBoardListing && !scope.HasGrace
	slice := scope.SliceBoardListing && scope.HasGrace
	if scope.FullCatalog || (!complete && !slice) {
		return nil
	}
	boards := slices.Clone(stats.QualifyingBoards)
	if complete && scope.EmptyBoardListing {
		boards = append(boards, stats.EmptyBoards...)
	}
	if !scope.RegionScoped {
		for i := range boards {
			boards[i].Region = ""
		}
	}
	slices.SortFunc(boards, func(a, b pipeline.BoardRef) int {
		return cmp.Or(strings.Compare(a.Board, b.Board), strings.Compare(a.Region, b.Region))
	})
	boards = slices.Compact(boards)
	boards = slices.DeleteFunc(boards, func(b pipeline.BoardRef) bool { return crossShardAmbiguous[b.Board] })
	if len(boards) == 0 {
		return nil
	}
	return boards
}

// SweepableCompanies narrows the crawled-company scope to the companies this run may actually
// close within: those it wrote a job for (crawled), minus the ones a board withheld because its
// crawl did not read what it listed (pipeline.Stats.UnprovenCompanies).
//
// The subtraction is what keeps the two halves of the fix honest. A board whose detail requests
// died still WRITES everything it did manage to read — those rows' last_seen_at moves, their
// content is refreshed, their company enters the crawled set exactly as before — and only the
// permission to retire that company's unseen rows is withheld. Recording and closing are
// separate decisions, and only the second one lacks the evidence.
//
// It withholds by COMPANY rather than by board because the crawled scope is the only handle the
// provider-wide CloseUnseenJobs has; a company under two boards, one of which read cleanly, is
// still withheld, which is the conservative reading and the correct one — the other board's
// unread postings belong to that same company.
//
// Returns nil rather than an empty slice when nothing is left, which the close reads as an
// empty text[] and matches no row (`company_slug = ANY('{}')`), i.e. it closes nothing.
//
// A failed covered refresh (Stats.RefreshFailed) withholds the whole scope. Those rows keep their
// old last_seen_at while the listing named them, their company can still be in the crawled set
// through another board's refresh, and a covered row is never written again to reopen it.
func SweepableCompanies(crawled []string, stats pipeline.Stats) []string {
	if stats.RefreshFailed > 0 {
		return nil
	}
	if len(stats.UnprovenCompanies) == 0 {
		return crawled
	}
	unproven := make(map[string]bool, len(stats.UnprovenCompanies))
	for _, slug := range stats.UnprovenCompanies {
		unproven[slug] = true
	}
	kept := slices.DeleteFunc(slices.Clone(crawled), func(slug string) bool { return unproven[slug] })
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// SweepableProviders returns, sorted, the providers with company/source eligibility or
// finished-board evidence. Board evidence only admits a provider to dispatch: callers
// retain ShouldSweep for company/source scope and SweepableBoards for board scope.
func SweepableProviders(rs pipeline.RunStats) []string {
	providers := make([]string, 0, len(rs))
	for provider, s := range rs {
		if ShouldSweep(s) || len(s.QualifyingBoards) > 0 || len(s.EmptyBoards) > 0 {
			providers = append(providers, provider)
		}
	}
	sort.Strings(providers)
	return providers
}

// ShouldSweep reports whether a run permits company/source sweeps. A run with no
// ingested or refreshed rows may still have independently verified board evidence,
// which SweepableBoards handles without granting this broader scope. A stored row the
// coverage gate refreshed counts too: on an aggregator whose every employer is crawled
// first-hand nothing is ingested, yet the crawl plainly worked, and those rows' companies are
// already in the crawled scope waiting for this sweep.
func ShouldSweep(stats pipeline.Stats) bool {
	return stats.Ingested > 0 || stats.Refreshed > 0
}

// SweepBySource reports whether a provider's run may close its unseen jobs by source alone,
// dropping the crawled-company scope. Only a fullCatalog source qualifies (it lists the whole
// catalogue each run, so an unseen job is truly gone), and only when the run had zero board
// failures: a fullCatalog adapter errors a truncated crawl, so Failed>0 means the listing was
// incomplete and a source-scoped close would mass-close the postings it never reached. Such a run
// falls back to the safe company-scoped CloseUnseenJobs. Callers gate on ShouldSweep first.
//
// A posting the crawl could not READ disqualifies it too, and here with no tolerance at all
// (unlike the board scope's maxUnreadablePercent): this is the broadest close in the system, it
// drops the company scope that SweepableCompanies uses to withhold anything, and no fullCatalog
// adapter emits the marker today — so the strict reading costs nothing now and is the one that
// stays right if one ever does.
//
// So does a board crawled without the catalogue's seen-set (Stats.SeenUnknown): every stored
// posting then reads as new, and a hydrating adapter with a detail budget refreshes none of
// those past it, so the source scope would close live postings the listing named.
//
// And so does a covered posting whose refresh failed (Stats.RefreshFailed): the listing named
// it, but its stored row keeps its old last_seen_at, and a covered row is never written again,
// so a wrong close would never be undone by a later crawl.
func SweepBySource(stats pipeline.Stats, fullCatalog bool) bool {
	return fullCatalog && stats.Failed == 0 && stats.Unreadable == 0 && stats.SeenUnknown == 0 &&
		stats.RefreshFailed == 0
}

// SweepWindowFor reports how long a provider's unseen jobs are spared before the sweep closes
// them: the window its adapter declared (sources.SweepGraceWindows), wider or narrower than
// staleAfter, else staleAfter itself. The override is per-provider so one feed's drift tolerance
// (or one full-board crawl's tighter cadence) never changes the sweep for every other provider.
func SweepWindowFor(grace map[string]time.Duration, provider string) time.Duration {
	if w, ok := grace[provider]; ok {
		return w
	}
	return StaleAfter
}
