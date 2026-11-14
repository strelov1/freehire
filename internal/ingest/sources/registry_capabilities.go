package sources

import (
	"slices"
	"time"
)

// SelfClosingProviders returns the provider names in reg that manage their own closes and
// must be skipped by the post-run unseen sweep (see selfClosing). cmd/ingest consults this
// when deciding which providers to sweep.
func SelfClosingProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(selfClosing); ok {
			out = append(out, name)
		}
	}
	return out
}

// SelfCloses reports whether src manages its own closes from a change stream (see selfClosing).
// Such a source emits CHANGES rather than its listing, so nothing derived from one crawl's
// emissions — the quiet-board backoff's listing fingerprint, for one — describes what the board
// still lists.
func SelfCloses(src Source) bool {
	_, ok := src.(selfClosing)
	return ok
}

// FullCatalogProviders returns the provider names in reg that list their whole catalogue each
// run and so may be swept by source rather than by crawled company (see fullCatalog).
func FullCatalogProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(fullCatalog); ok {
			out = append(out, name)
		}
	}
	return out
}

// SliceBoardListingProviders returns, as a set, the provider names in reg whose boards are
// bounded slices the board-scoped close may act on over the provider's sweepGrace window (see
// sliceBoardListing).
func SliceBoardListingProviders(reg map[string]Source) map[string]bool {
	out := make(map[string]bool)
	for name, src := range reg {
		if _, ok := src.(sliceBoardListing); ok {
			out[name] = true
		}
	}
	return out
}

// SweepGraceWindows returns the post-run sweep window each adapter in reg declares in place of
// the default — wider for a slice-crawled source that can't yet tell drift from removal, narrower
// for a full-board source whose unseen reading is already evidence (see sweepGrace). cmd/ingest
// consults this when computing a provider's cutoff; a provider absent from the map is swept on
// the default window.
func SweepGraceWindows(reg map[string]Source) map[string]time.Duration {
	out := make(map[string]time.Duration)
	for name, src := range reg {
		if s, ok := src.(sweepGrace); ok {
			out[name] = s.sweepGrace()
		}
	}
	return out
}

// UnseenWindows returns, per provider, how long one of its postings may go unseen by the
// crawl before search stops showing it (the safety net for postings the post-run sweep never
// closed, because no run proved it covered their board). Every crawled provider gets floor,
// or its own sweepGrace when that is wider — a source whose sweep already waits longer has
// said its unseen reading means nothing sooner. A provider is absent, and its postings are
// never withheld on this ground, when last_seen_at going stale is not evidence for it:
//
//   - a self-closing adapter (see selfClosing) re-reports only CHANGED postings, so an
//     untouched live one ages exactly like a removed one;
//   - a livenessProbed adapter re-lists only the newest slice of its feed, and a probe of the
//     source's own page closes what is really gone.
//
// A source with no adapter at all — Telegram, an employer's own vacancy, a moderator's or a
// pasted-link import — is absent by construction: nothing re-crawls it, so its last_seen_at
// never moves. reg must be Taxonomy(), so the answer does not vary with a host's credentials.
func UnseenWindows(reg map[string]Source, floor time.Duration) map[string]time.Duration {
	out := make(map[string]time.Duration, len(reg))
	for name, src := range reg {
		if _, ok := src.(selfClosing); ok {
			continue
		}
		if _, ok := src.(livenessProbed); ok {
			continue
		}
		window := floor
		if s, ok := src.(sweepGrace); ok {
			window = max(window, s.sweepGrace())
		}
		out[name] = window
	}
	return out
}

// FullBoardListingProviders returns, as a set, the provider names in reg whose adapter is
// registered as structurally proving it lists a board to completion (see fullBoardListing).
// cmd/ingest consults this as the sweep's fourth board-scope qualification gate: a provider
// absent from this set never contributes to the board-scoped close, however its crawl went.
func FullBoardListingProviders(reg map[string]Source) map[string]bool {
	out := make(map[string]bool)
	for name, src := range reg {
		if _, ok := src.(fullBoardListing); ok {
			out[name] = true
		}
	}
	return out
}

// EmptyBoardListingProviders returns the provider names in reg whose adapter carries the
// emptyBoardListing marker: a finished crawl of one of their boards that listed nothing is
// evidence the board is empty, so cmd/ingest admits it to the board-scoped close.
func EmptyBoardListingProviders(reg map[string]Source) map[string]bool {
	out := make(map[string]bool)
	for name, src := range reg {
		if _, ok := src.(emptyBoardListing); ok {
			out[name] = true
		}
	}
	return out
}

// ScopesBoardsByRegion reports whether src narrows a board to one region by its posting URLs
// (RegionScopedBoards), so a board name repeated across its regions is not ambiguous.
func ScopesBoardsByRegion(src Source) bool {
	_, ok := src.(RegionScopedBoards)
	return ok
}

// BoardURLPrefix returns the prefix every posting URL of src's board in region starts with, for
// an adapter that scopes boards by region. ok is false for any other adapter, and for a region
// the adapter does not know.
func BoardURLPrefix(src Source, region string) (string, bool) {
	rs, ok := src.(RegionScopedBoards)
	if !ok {
		return "", false
	}
	return rs.BoardURLPrefix(region)
}

// AggregatorProviders returns the sorted provider names in reg that aggregate postings
// from many companies (see aggregator). The cross-source dedup pass uses this to tell an
// aggregator copy (which may be suppressed) from a first-party ATS posting (which wins).
func AggregatorProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, ok := src.(aggregator); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// IsAggregator reports whether reg's adapter for provider carries the aggregator marker —
// the question the dedup, coverage and overlap passes ask, which is not the same as
// ProviderKind == KindAggregator: a social adapter (hackernews) is an aggregator to them and
// social to a reader. reg must be Taxonomy(), for the reason ProviderKind gives.
func IsAggregator(reg map[string]Source, provider string) bool {
	_, ok := reg[provider].(aggregator)
	return ok
}

// nationalMarket is an aggregator that lists one country's job market, so the coverage gate
// credits an employer's own board only where that board hires in the same country. Without
// it the gate matched by company name alone: a Slovenian "Arriva d.o.o." on mojedelo was
// discarded because the UK Arriva's Workday board is crawled, and "Echo center d.o.o." because
// an unrelated US "echo" is (issue #4412, ~400 of ~3,700 mojedelo postings a run).
type nationalMarket interface {
	marketCountries(e CompanyEntry) []string
}

// MarketCountries returns the lowercase alpha-2 countries the board e lists for an aggregator
// scoped to a national market, or nil for one that is not (the gate then stays country-blind).
// reg must be Taxonomy(), as for IsAggregator.
func MarketCountries(reg map[string]Source, e CompanyEntry) []string {
	if m, ok := reg[e.Provider].(nationalMarket); ok {
		return m.marketCountries(e)
	}
	return nil
}

// BoardKeyedProviders returns the sorted provider names in reg whose adapter is addressed by
// a board id — every adapter that is not boardless. A caller that wants to say something about
// ONE board (cmd/harvest-boards validating a candidate) needs this: a boardless adapter serves
// the same catalogue whatever board it is handed, so it would answer for a board that does not
// exist, and confirm every candidate put to it.
func BoardKeyedProviders(reg map[string]Source) []string {
	var out []string
	for name, src := range reg {
		if _, boardless := src.(boardless); !boardless {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// FilterableProviders returns the sorted provider keys the source facet offers.
func FilterableProviders() []string { return filterableProviders(Taxonomy()) }

// filterableProviders selects the source-facet provider keys from a registry. A
// single-company boardless platform is redundant with the company filter and excluded;
// a board-based platform or a multi-company aggregator stays listed.
func filterableProviders(registry map[string]Source) []string {
	var out []string
	for key, src := range registry {
		if _, isBoardless := src.(boardless); isBoardless {
			if _, isAggregator := src.(aggregator); !isAggregator {
				continue
			}
		}
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
