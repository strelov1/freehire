package sweep

import (
	"reflect"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// Provider dispatch includes independently verified board evidence even when no job
// was ingested. A failed crawl with neither kind of evidence remains excluded.
func TestSweepableProviders(t *testing.T) {
	rs := pipeline.RunStats{
		"vk":         {Ingested: 5},
		"personio":   {EmptyBoards: refs("emptied")},
		"board-only": {Rejected: 2, QualifyingBoards: refs("read-completely")},
		"ozon":       {Ingested: 0, Failed: 3}, // crawl failed → excluded
		"sber":       {Ingested: 2},
	}
	got := SweepableProviders(rs)
	want := []string{"board-only", "personio", "sber", "vk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SweepableProviders = %v, want %v", got, want)
	}
}

// The company/source guard remains closed for board-only evidence: dispatching such
// a provider must not grant a whole-source sweep.
func TestShouldSweep(t *testing.T) {
	cases := []struct {
		name  string
		stats pipeline.Stats
		want  bool
	}{
		{"normal run", pipeline.Stats{Ingested: 100, Failed: 3}, true},
		{"zero ingested", pipeline.Stats{Ingested: 0, Failed: 550}, false},
		{"all boards ok but empty", pipeline.Stats{Ingested: 0, Failed: 0}, false},
		{"empty-board evidence is board scope only", pipeline.Stats{EmptyBoards: refs("emptied")}, false},
		{"qualifying-board evidence is board scope only", pipeline.Stats{QualifyingBoards: refs("read-completely")}, false},
		// Every posting went to an employer crawled first-hand, and the stored ones among them
		// were refreshed: the crawl worked, so the companies those rows put in scope must sweep.
		{"covered only, stored rows refreshed", pipeline.Stats{ATSCovered: 40, Refreshed: 12}, true},
		{"covered only, nothing stored", pipeline.Stats{ATSCovered: 40}, false},
	}
	for _, tc := range cases {
		if got := ShouldSweep(tc.stats); got != tc.want {
			t.Errorf("%s: ShouldSweep = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A slice-crawled source declares a window wider than the default so a posting that merely
// drifted past the crawl's page depth is not closed and then reopened; every other provider
// keeps the default untouched.
func TestSweepWindowFor(t *testing.T) {
	grace := map[string]time.Duration{"whatjobs": 14 * 24 * time.Hour}

	cases := []struct {
		name     string
		provider string
		want     time.Duration
	}{
		{"declaring provider gets its wider window", "whatjobs", 14 * 24 * time.Hour},
		{"ordinary provider keeps the default", "greenhouse", StaleAfter},
	}
	for _, tc := range cases {
		if got := SweepWindowFor(grace, tc.provider); got != tc.want {
			t.Errorf("%s: SweepWindowFor(%q) = %v, want %v", tc.name, tc.provider, got, tc.want)
		}
	}
}

// A fullCatalog source may close by source only after a clean (zero-Failed) run: a truncated
// crawl looks like a shrunken catalogue and a source-scoped close would mass-close the postings
// it never reached, so a partial run must fall back to the company-scoped close.
func TestSweepBySource(t *testing.T) {
	cases := []struct {
		name        string
		stats       pipeline.Stats
		fullCatalog bool
		want        bool
	}{
		{"full-catalog, clean run", pipeline.Stats{Ingested: 1000, Failed: 0}, true, true},
		{"full-catalog, a board failed", pipeline.Stats{Ingested: 1000, Failed: 1}, true, false},
		{"not full-catalog, clean run", pipeline.Stats{Ingested: 1000, Failed: 0}, false, false},
		// The source scope drops the company scope entirely, so it is the one close
		// SweepableCompanies cannot narrow — a single unread posting disqualifies it.
		{"full-catalog, one posting unread", pipeline.Stats{Ingested: 1000, Unreadable: 1}, true, false},
		// Without its seen-set a budgeted adapter refreshes nothing it already stored, so
		// every stored posting would read as gone.
		{"full-catalog, seen-set unknown", pipeline.Stats{Ingested: 1000, SeenUnknown: 1}, true, false},
		// A covered posting the listing named but whose refresh failed keeps its old
		// last_seen_at, and the source scope would read that as gone.
		{"full-catalog, covered refresh failed", pipeline.Stats{Ingested: 1000, RefreshFailed: 3}, true, false},
	}
	for _, tc := range cases {
		if got := SweepBySource(tc.stats, tc.fullCatalog); got != tc.want {
			t.Errorf("%s: SweepBySource = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The company-scoped close asks "which companies did this run see enough of to retire their
// unseen postings", and writing a job is no longer the whole answer: a board whose detail
// requests died wrote everything it could read while missing postings that are still live, so
// its companies are subtracted here. Recording and closing are separate decisions, and only
// the second one lacks the evidence.
// A failed covered refresh leaves listed rows with their old last_seen_at. The company scope
// could still name such a row's company through another board's refresh, and RefreshCoveredJobs
// never reopens what it would wrongly close, so the run withholds the company scope too.
func TestSweepableCompaniesWithheldAfterAFailedCoveredRefresh(t *testing.T) {
	got := SweepableCompanies([]string{"acme", "globex"}, pipeline.Stats{Ingested: 5, Refreshed: 3, RefreshFailed: 2})
	if got != nil {
		t.Errorf("SweepableCompanies = %v, want nil", got)
	}
}

func TestSweepableCompanies(t *testing.T) {
	cases := []struct {
		name    string
		crawled []string
		stats   pipeline.Stats
		want    []string
	}{
		{
			name:    "nothing withheld leaves the crawled scope untouched",
			crawled: []string{"acme", "globex"},
			want:    []string{"acme", "globex"},
		},
		{
			name:    "a company whose board could not read what it listed is withheld",
			crawled: []string{"acme", "globex"},
			stats:   pipeline.Stats{UnprovenCompanies: []string{"globex"}},
			want:    []string{"acme"},
		},
		{
			// A company under two boards, one of which read cleanly, is still withheld: the
			// other board's unread postings belong to that same company, and the close cannot
			// tell them apart.
			name:    "one withheld board withholds the company its other board also crawled",
			crawled: []string{"acme"},
			stats:   pipeline.Stats{UnprovenCompanies: []string{"acme", "acme"}},
			want:    nil,
		},
		{
			// nil rather than an empty slice, but either way the close's `= ANY($slugs)`
			// matches no row — the point is that it closes NOTHING, never everything.
			name:    "withholding every crawled company leaves no scope at all",
			crawled: []string{"acme"},
			stats:   pipeline.Stats{UnprovenCompanies: []string{"acme"}},
			want:    nil,
		},
		{
			name:    "a withheld company this run never wrote for changes nothing",
			crawled: []string{"acme"},
			stats:   pipeline.Stats{UnprovenCompanies: []string{"globex"}},
			want:    []string{"acme"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SweepableCompanies(tc.crawled, tc.stats)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SweepableCompanies(%v, %v) = %v, want %v", tc.crawled, tc.stats.UnprovenCompanies, got, tc.want)
			}
		})
	}
}

// The board-scoped close (freehire#2328) only ever touches a board the provider's adapter is
// registered as listing to completion, and is withheld from a provider excluded for the same
// reasons SweepBySource withholds the source-scoped close: a sweepGrace provider (its crawl
// deliberately reaches only a slice) or a fullCatalog provider (already closes by source alone,
// strictly broader — belt-and-braces since today's fullCatalog adapters are boardless anyway).
// Duplicate board entries (a repeated board-file row, or one board id recurring across regional
// slices) are de-duplicated so neither the close nor its log line double-counts.
func TestSweepableBoards(t *testing.T) {
	noneAmbiguous := map[string]bool{}
	cases := []struct {
		name                string
		stats               pipeline.Stats
		scope               BoardScope
		crossShardAmbiguous map[string]bool
		want                []pipeline.BoardRef
	}{
		{
			name:  "registered provider's qualifying boards are returned sorted",
			scope: BoardScope{FullBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("zeta-inc", "acme-corp")},
			want:  refs("acme-corp", "zeta-inc"),
		},
		{
			name:  "duplicate boards are de-duplicated",
			scope: BoardScope{FullBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("acme-corp", "acme-corp")},
			want:  refs("acme-corp"),
		},
		{
			name:  "a provider not registered as fullBoardListing gets none",
			stats: pipeline.Stats{QualifyingBoards: refs("acme-corp")},
			want:  nil,
		},
		{
			// Distinguishes hasGrace from fullCatalog independently (both otherwise zero the
			// result the same way), so a future edit transposing them at the call site fails.
			name:  "a sweepGrace provider is excluded even if registered, and not for the fullCatalog reason",
			scope: BoardScope{HasGrace: true, FullCatalog: false, FullBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("acme-corp")},
			want:  nil,
		},
		{
			name:  "a fullCatalog provider is excluded even if registered, and not for the sweepGrace reason",
			scope: BoardScope{HasGrace: false, FullCatalog: true, FullBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("acme-corp")},
			want:  nil,
		},
		{
			// whatjobs: a keyword board read to a page budget never proves a full listing, but its
			// grace window absorbs the drift past that budget, so its finished boards close on it.
			name:  "a sliceBoardListing provider's boards are returned on its grace window",
			scope: BoardScope{HasGrace: true, SliceBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("data engineer")},
			want:  refs("data engineer"),
		},
		{
			// Without a grace window the slice marker would close on the 48h default whatever sat
			// past the page budget for two days.
			name:  "a sliceBoardListing provider without a grace window gets none",
			scope: BoardScope{SliceBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("data engineer")},
			want:  nil,
		},
		{
			// The cross-shard case (freehire#2328 review): a board name this provider's run
			// saw as unambiguous (it only ever crawled one of its regions) can still be
			// region-ambiguous across the FULL, unsharded catalog — sources.Config.Shard
			// groups by company slug, not board, so the other region may be a different
			// shard's problem entirely. SweepableBoards must refuse it regardless.
			name:                "a board ambiguous across the full unsharded catalog is excluded even though this run's own Stats saw it as unambiguous",
			scope:               BoardScope{FullBoardListing: true},
			stats:               pipeline.Stats{QualifyingBoards: refs("acme-corp")},
			crossShardAmbiguous: map[string]bool{"acme-corp": true},
			want:                nil,
		},
		{
			// An employer that took every posting down: the run finished the board and its
			// listing named nothing. An adapter that answers an emptied board distinguishably from
			// a failed fetch closes it like any other finished board.
			name:  "an emptyBoardListing provider's empty boards join its qualifying ones",
			scope: BoardScope{FullBoardListing: true, EmptyBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("zeta-inc"), EmptyBoards: refs("acme-corp")},
			want:  refs("acme-corp", "zeta-inc"),
		},
		{
			// fullBoardListing alone proves a listing was read to its end, not that an empty
			// answer was a listing at all — a challenge page parses to nothing too.
			name:  "a provider without emptyBoardListing gets none of its empty boards",
			scope: BoardScope{FullBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: refs("zeta-inc"), EmptyBoards: refs("acme-corp")},
			want:  refs("zeta-inc"),
		},
		{
			// Workday's own answer for an emptied site is a 200 with jobPostings: [], so the
			// ordinary board scope closes it once its postings are past the grace window.
			name:  "an emptied workday board is swept",
			scope: boardScopeFor(t, "workday"),
			stats: pipeline.Stats{EmptyBoards: refs("acme.wd1.myworkdayjobs.com/Careers")},
			want:  refs("acme.wd1.myworkdayjobs.com/Careers"),
		},
		{
			name:  "an empty board needs a complete listing, not a slice",
			scope: BoardScope{HasGrace: true, SliceBoardListing: true, EmptyBoardListing: true},
			stats: pipeline.Stats{EmptyBoards: refs("data engineer")},
			want:  nil,
		},
		{
			name:                "an empty board ambiguous across the unsharded catalog is excluded",
			scope:               BoardScope{FullBoardListing: true, EmptyBoardListing: true},
			stats:               pipeline.Stats{EmptyBoards: refs("acme-corp")},
			crossShardAmbiguous: map[string]bool{"acme-corp": true},
			want:                nil,
		},
		{
			// jobstreet: subclass "6290" is a board in every market, so each finished (board,
			// region) is its own scope, and the region survives to narrow the close by url.
			name:  "a region-scoped provider keeps each board's region",
			scope: BoardScope{FullBoardListing: true, RegionScoped: true},
			stats: pipeline.Stats{QualifyingBoards: []pipeline.BoardRef{{Board: "6290", Region: "sg"}, {Board: "6290", Region: "my"}, {Board: "6290", Region: "sg"}}},
			want:  []pipeline.BoardRef{{Board: "6290", Region: "my"}, {Board: "6290", Region: "sg"}},
		},
		{
			// seek: a finished slice closes on the grace window and keeps its market, since
			// subclassification "6290" is a board in AU and NZ alike.
			name:  "a region-scoped slice provider keeps each finished slice's region",
			scope: BoardScope{HasGrace: true, SliceBoardListing: true, RegionScoped: true},
			stats: pipeline.Stats{QualifyingBoards: []pipeline.BoardRef{{Board: "6290", Region: "nz"}, {Board: "6290", Region: "au"}}},
			want:  []pipeline.BoardRef{{Board: "6290", Region: "au"}, {Board: "6290", Region: "nz"}},
		},
		{
			// A slice that failed, was interrupted or read too little never reaches
			// QualifyingBoards, so it must not be swept.
			name:  "a slice that did not qualify is not swept",
			scope: BoardScope{HasGrace: true, SliceBoardListing: true, RegionScoped: true},
			stats: pipeline.Stats{QualifyingBoards: []pipeline.BoardRef{{Board: "6290", Region: "au"}}},
			want:  []pipeline.BoardRef{{Board: "6290", Region: "au"}},
		},
		{
			name:  "any other provider's boards are keyed on the id alone",
			scope: BoardScope{FullBoardListing: true},
			stats: pipeline.Stats{QualifyingBoards: []pipeline.BoardRef{{Board: "acme", Region: "us"}, {Board: "acme", Region: ""}}},
			want:  refs("acme"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ambiguous := tc.crossShardAmbiguous
			if ambiguous == nil {
				ambiguous = noneAmbiguous
			}
			got := SweepableBoards(tc.stats, tc.scope, ambiguous)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SweepableBoards() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBodyRefreshFor(t *testing.T) {
	// 2026-09-06 is day 249 of the year; 249 % 30 = 9.
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		days       string
		slice      string
		wantOn     bool
		wantSlot   int64
		wantSlices int64
		wantErr    bool
	}{
		{name: "unset is disabled", wantSlot: BodyRefreshDisabledSlot, wantSlices: 1},
		{name: "enabled with the default slice", days: "45", wantOn: true, wantSlot: 9, wantSlices: 30},
		{name: "enabled with a custom slice", days: "45", slice: "10", wantOn: true, wantSlot: 9, wantSlices: 10},
		{name: "one slice re-reads every stale row", days: "45", slice: "1", wantOn: true, wantSlot: 0, wantSlices: 1},
		{name: "days not a number", days: "six weeks", wantErr: true},
		{name: "days zero", days: "0", wantErr: true},
		{name: "days negative", days: "-1", wantErr: true},
		{name: "slice not a number", days: "45", slice: "half", wantErr: true},
		{name: "slice zero", days: "45", slice: "0", wantErr: true},
		// A slice set without days is a knob that would silently do nothing, which reads
		// exactly like a refresh that found nothing to refresh.
		{name: "slice without days", slice: "10", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BodyRefreshFor(tc.days, tc.slice, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("BodyRefreshFor(%q, %q) err = %v, wantErr %v", tc.days, tc.slice, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got.Enabled() != tc.wantOn {
				t.Errorf("Enabled() = %v, want %v", got.Enabled(), tc.wantOn)
			}
			if got.Slot != tc.wantSlot {
				t.Errorf("slot = %d, want %d", got.Slot, tc.wantSlot)
			}
			if got.Slices != tc.wantSlices {
				t.Errorf("slices = %d, want %d", got.Slices, tc.wantSlices)
			}
		})
	}
}

// The disabled form must make the SQL arm a no-op through the ordinary parameters rather than
// through a second query: a slot no row can hash to. Every other value it carries is then
// irrelevant, which is the point.
func TestBodyRefreshDisabledSlotMatchesNoRow(t *testing.T) {
	off, err := BodyRefreshFor("", "", time.Now())
	if err != nil {
		t.Fatalf("BodyRefreshFor: %v", err)
	}
	if off.Slices < 1 {
		t.Errorf("slices = %d, want a positive divisor even when disabled (a zero modulus raises)", off.Slices)
	}
	// abs(hashtext(x)) % slices is never negative, so a negative slot cannot be matched.
	if off.Slot >= 0 {
		t.Errorf("slot = %d, want a negative slot so no row is ever withheld", off.Slot)
	}
}

// refs is boards without a region, the shape every provider but a region-scoped one reports.
func refs(boards ...string) []pipeline.BoardRef {
	if len(boards) == 0 {
		return nil
	}
	out := make([]pipeline.BoardRef, len(boards))
	for i, b := range boards {
		out[i] = pipeline.BoardRef{Board: b}
	}
	return out
}

func TestWithDetailReread(t *testing.T) {
	// 2026-11-04 is day 308 of the year; 308 % 7 = 0.
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	off, err := BodyRefreshFor("", "", now)
	if err != nil {
		t.Fatalf("BodyRefreshFor: %v", err)
	}

	got := off.WithDetailReread("ess", now)
	want := BodyRefresh{Cutoff: detailFieldAdded["ess"], Slices: detailRereadSlices, Slot: 0}
	if got != want {
		t.Errorf("listed provider = %+v, want %+v", got, want)
	}

	if got := off.WithDetailReread("greenhouse", now); got.Enabled() {
		t.Errorf("unlisted provider = %+v, want disabled", got)
	}

	// A row that can never be stamped must not come back for as long as the source lists it.
	over := detailFieldAdded["ess"].Add(detailRereadWindow + time.Hour)
	if got := off.WithDetailReread("ess", over); got.Enabled() {
		t.Errorf("past the window = %+v, want disabled", got)
	}

	// An operator's BODY_REFRESH_DAYS is not overridden.
	set, err := BodyRefreshFor("45", "", now)
	if err != nil {
		t.Fatalf("BodyRefreshFor: %v", err)
	}
	if got := set.WithDetailReread("ess", now); got != set {
		t.Errorf("provider with its own refresh = %+v, want %+v", got, set)
	}
}

// An entry naming a provider that does not exist would re-read nothing and look finished.
func TestDetailFieldAddedNamesRealHydratingProviders(t *testing.T) {
	registry := sources.All(sources.NewClient())
	for provider := range detailFieldAdded {
		src, ok := registry[provider]
		if !ok {
			t.Errorf("%q is not a registered provider", provider)
			continue
		}
		if _, ok := src.(sources.HydratingSource); !ok {
			t.Errorf("%q is not a HydratingSource, so withholding its rows changes nothing", provider)
		}
	}
}

func boardScopeFor(t *testing.T, provider string) BoardScope {
	t.Helper()
	reg := sources.All(nil)
	return BoardScope{
		FullBoardListing:  sources.FullBoardListingProviders(reg)[provider],
		EmptyBoardListing: sources.EmptyBoardListingProviders(reg)[provider],
	}
}
