// Package kit is the shared kernel of the source adapters: the Source contract, the Job
// shape, the HTTP client interfaces and the parsing helpers every adapter family uses. It
// imports nothing of sources, so a family subpackage (sources/<family>) can build on it while
// the registry in sources imports the family.
package kit

import (
	"context"
	"fmt"
	"time"
)

// DefaultSweepGrace is the grace window a provider is swept on when it declares no
// sweepGrace of its own: many crawl cycles at the hourly per-provider cadence, so a board
// failing several runs in a row keeps its jobs open. cmd/ingest's own unseen sweep and
// cmd/liveness's probeDespiteRegistered backstop (which only picks up what that sweep has
// already had a chance to close) both anchor on this single symbol so the two windows
// cannot drift apart silently.
const DefaultSweepGrace = 48 * time.Hour

// CompanyEntry is one configured board from a board file (sources/<provider>.yml): the
// company whose jobs we crawl, the platform it uses (Provider), and the platform-specific
// board id. Region is an optional per-entry hint for ATS platforms that host tenants on
// regional API domains (e.g. Lever's EU data-residency host); empty means the default host.
// Hub is an optional per-entry flag marking a board as a community/agency hub whose vacancies
// belong to many partner companies; an adapter that honours it resolves each job's employer from
// the posting and uses Company only as the hub name and per-vacancy fallback (e.g. huntflow's
// AlumniHub). It is ignored by adapters that do not implement hub resolution.
// Tenants is the companion map for a hub whose postings identify their tenant only by an opaque
// key — a URL path segment or an id — that is not itself a usable company name; it maps that key
// to the employer's display name (e.g. successfactors' "Arvato_Systems" → "Arvato Systems").
// A key absent from the map falls back to Company rather than being turned into a name by
// transforming the key, because a plausible-but-wrong employer reads worse in the catalogue than
// the parent brand. Optional and adapter-specific, exactly as Hub and Region are.
type CompanyEntry struct {
	Company  string            `yaml:"company"`
	Provider string            `yaml:"provider"`
	Board    string            `yaml:"board"`
	Region   string            `yaml:"region"`
	Hub      bool              `yaml:"hub"`
	Tenants  map[string]string `yaml:"tenants"`
}

type ApplicationMethod string

const (
	ApplyOnSite   ApplicationMethod = "onsite"
	ApplyExternal ApplicationMethod = "external"
)

// Source adapts one job-source platform. Provider is the platform key that selects
// the adapter (it matches CompanyEntry.Provider and the stored jobs.source); Fetch
// returns all current postings for one configured board.
type Source interface {
	Provider() string
	Fetch(ctx context.Context, e CompanyEntry) ([]Job, error)
}

// StreamingSource is a Source that can also stream its postings to a sink as it crawls, so the
// pipeline persists them incrementally rather than buffering the whole board until Fetch
// returns. An adapter with an expensive per-posting detail fan-out (taleo, whose large
// catalogues take many minutes to crawl) implements it so a long crawl's
// progress is saved as it goes — partial work survives an interrupted or rate-limited run, and
// the catalogue converges across runs. emit is called once per ready posting and may be called
// concurrently. FetchStream returns an error only for a board-level failure (e.g. the listing
// failed); a single dropped posting is simply not emitted.
type StreamingSource interface {
	Source
	FetchStream(ctx context.Context, e CompanyEntry, emit func(Job)) error
}

// HydratingSource is a Source that fetches expensive per-posting detail (e.g. the description
// the list omits) only for postings the catalogue does not already have. The pipeline supplies a
// seen predicate — seen(externalID) reports whether that posting is already ingested for the
// provider — so a large aggregator (justjoin) issues detail requests only for new postings
// instead of on every crawl. An adapter opts in by implementing this in addition to Fetch (the
// list-only fallback used when the pipeline cannot supply a seen set); every other adapter is
// unaffected. The pipeline prefers FetchNew when the adapter implements it.
type HydratingSource interface {
	Source
	FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error)
}

// RecentSource is a Source whose platform can filter a listing by publication date, so a run
// can ask for only the newest postings. FetchRecent returns the postings published within the
// last window (a whole number of days) and nothing older. It NEVER signals a removal: a recent listing omits every older posting
// by construction, so the pipeline treats the result as no evidence about what is gone (see
// pipeline.Runner.RecentDays).
//
// An adapter opts in only when the source documents a date filter that was verified against it.
// An adapter that does not implement this is skipped in a recent run rather than crawled in full.
type RecentSource interface {
	Source
	FetchRecent(ctx context.Context, e CompanyEntry, window time.Duration) ([]Job, error)
}

// CoverageGated is a HydratingSource that can also skip a posting's detail when the pipeline is
// going to discard that posting anyway — the aggregator-coverage gate because the employer is
// already covered by a non-aggregator source.
//
// It exists because `seen` alone cannot bound the work on such a board. A discarded posting is
// never stored, so it is never seen, so the ordinary hydrating crawl pays for its body on
// EVERY run and throws the body away every time. That is minor on most aggregators and the
// whole board on one that lists many already-covered postings (remote.com).
//
// The production pipeline supplies no decisions today: its Covered callback always returns no
// companies (another vacancy at an employer is not duplicate evidence, #4764), and nothing is
// refused on title (#4412), so a gated adapter hydrates like any other.
//
// The contract is about COST, not about the result: a posting the gate condemns is still
// yielded, just without a body. Dropping it instead would be the adapter quietly making the
// pipeline's decision, which is the pipeline's to make and to count.
type CoverageGated interface {
	HydratingSource
	FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error)
}

// DetailGate is what the pipeline can say about a listed posting BEFORE its body is bought:
// whether the posting is going to be discarded anyway. Both answers come from the pipeline's
// own rules, so an adapter never restates catalogue policy — it only decides what to spend.
type DetailGate struct {
	// Covered is called ONCE with every company the crawl listed and answers which of them are
	// already covered by a non-aggregator source, keyed by the SAME strings passed in — the
	// adapter states company names and never has to know how freehire slugs them.
	Covered func(companies []string) map[string]bool
}

// CompanyDescriber is a Source that can also fetch its board's own company-level
// description — an "about us" field the platform exposes separately from posting
// bodies (e.g. Greenhouse's board-metadata endpoint) — once per board rather than
// once per posting. The pipeline calls it after a board's Fetch, and treats a
// ("", nil) return, an error, or the interface simply not being implemented the
// same way: no description, no company-info write for that crawl. Optional because
// most platforms expose no such field at all.
type CompanyDescriber interface {
	Source
	CompanyDescription(ctx context.Context, e CompanyEntry) (string, error)
}

// RegionScopedBoards marks an adapter whose board id repeats across REGIONS of one provider
// (boards' UNIQUE(provider, lower(board), region)) while the stored external_id carries no
// region — jobstreet's subclass "6290" is a board in six markets. The board-scoped close keys on
// external_id alone, so such a board name is refused as ambiguous (pipeline.ambiguousRegionBoards)
// unless the adapter can say where one region's postings live: BoardURLPrefix returns the prefix
// every posting URL of that region starts with, which the close adds as a url predicate. It must
// be exact — a prefix shared by two regions would let one region's crawl close the other's rows —
// and ok=false for a region it does not know. See the package-level BoardURLPrefix.
type RegionScopedBoards interface {
	BoardURLPrefix(region string) (prefix string, ok bool)
}

// ErrListingAbsent is the failed Fetch an emptyBoardListing adapter returns when its listing
// response decoded without the collection the source lists postings in. An empty board answers
// with the collection present and empty; an answer without it is not that answer, and treating
// it as one would close the board.
func ErrListingAbsent(provider, board, field string) error {
	return fmt.Errorf("%s: board %s: the listing response carries no %q collection", provider, board, field)
}
