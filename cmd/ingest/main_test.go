package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
)

// In a multi-provider run only the providers that ingested at least one job are swept,
// so a provider whose crawl failed (ingested 0) never has its catalogue mass-closed. The
// result is sorted for a deterministic sweep order.
func TestSweepableProviders(t *testing.T) {
	rs := pipeline.RunStats{
		"vk":   {Ingested: 5},
		"ozon": {Ingested: 0, Failed: 3}, // crawl failed → excluded
		"sber": {Ingested: 2},
	}
	got := sweepableProviders(rs)
	want := []string{"sber", "vk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sweepableProviders = %v, want %v", got, want)
	}
}

// The sweep guard: closing stale jobs is only safe after a run that actually saw
// postings — a zero-ingest run (total crawl outage) must not trigger the sweep.
func TestShouldSweep(t *testing.T) {
	cases := []struct {
		name  string
		stats pipeline.Stats
		want  bool
	}{
		{"normal run", pipeline.Stats{Ingested: 100, Failed: 3}, true},
		{"zero ingested", pipeline.Stats{Ingested: 0, Failed: 550}, false},
		{"all boards ok but empty", pipeline.Stats{Ingested: 0, Failed: 0}, false},
	}
	for _, tc := range cases {
		if got := shouldSweep(tc.stats); got != tc.want {
			t.Errorf("%s: shouldSweep = %v, want %v", tc.name, got, tc.want)
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
		{"ordinary provider keeps the default", "greenhouse", staleAfter},
	}
	for _, tc := range cases {
		if got := sweepWindowFor(grace, tc.provider); got != tc.want {
			t.Errorf("%s: sweepWindowFor(%q) = %v, want %v", tc.name, tc.provider, got, tc.want)
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
	}
	for _, tc := range cases {
		if got := sweepBySource(tc.stats, tc.fullCatalog); got != tc.want {
			t.Errorf("%s: sweepBySource = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The board-scoped close (freehire#2328) only ever touches a board the provider's adapter is
// registered as listing to completion, and is withheld from a provider excluded for the same
// reasons sweepBySource withholds the source-scoped close: a sweepGrace provider (its crawl
// deliberately reaches only a slice) or a fullCatalog provider (already closes by source alone,
// strictly broader — belt-and-braces since today's fullCatalog adapters are boardless anyway).
// Duplicate board entries (a repeated board-file row, or one board id recurring across regional
// slices) are de-duplicated so neither the close nor its log line double-counts.
func TestSweepableBoards(t *testing.T) {
	noneAmbiguous := map[string]bool{}
	cases := []struct {
		name                                    string
		stats                                   pipeline.Stats
		hasGrace, fullCatalog, fullBoardListing bool
		crossShardAmbiguous                     map[string]bool
		want                                    []string
	}{
		{
			name:             "registered provider's qualifying boards are returned sorted",
			stats:            pipeline.Stats{QualifyingBoards: []string{"zeta-inc", "acme-corp"}},
			fullBoardListing: true,
			want:             []string{"acme-corp", "zeta-inc"},
		},
		{
			name:             "duplicate boards are de-duplicated",
			stats:            pipeline.Stats{QualifyingBoards: []string{"acme-corp", "acme-corp"}},
			fullBoardListing: true,
			want:             []string{"acme-corp"},
		},
		{
			name:  "a provider not registered as fullBoardListing gets none",
			stats: pipeline.Stats{QualifyingBoards: []string{"acme-corp"}},
			want:  nil,
		},
		{
			// Distinguishes hasGrace from fullCatalog independently (both otherwise zero the
			// result the same way), so a future edit transposing them at the call site fails.
			name:             "a sweepGrace provider is excluded even if registered, and not for the fullCatalog reason",
			stats:            pipeline.Stats{QualifyingBoards: []string{"acme-corp"}},
			hasGrace:         true,
			fullCatalog:      false,
			fullBoardListing: true,
			want:             nil,
		},
		{
			name:             "a fullCatalog provider is excluded even if registered, and not for the sweepGrace reason",
			stats:            pipeline.Stats{QualifyingBoards: []string{"acme-corp"}},
			hasGrace:         false,
			fullCatalog:      true,
			fullBoardListing: true,
			want:             nil,
		},
		{
			// The cross-shard case (freehire#2328 review): a board name this provider's run
			// saw as unambiguous (it only ever crawled one of its regions) can still be
			// region-ambiguous across the FULL, unsharded catalog — sources.Config.Shard
			// groups by company slug, not board, so the other region may be a different
			// shard's problem entirely. sweepableBoards must refuse it regardless.
			name:                "a board ambiguous across the full unsharded catalog is excluded even though this run's own Stats saw it as unambiguous",
			stats:               pipeline.Stats{QualifyingBoards: []string{"acme-corp"}},
			fullBoardListing:    true,
			crossShardAmbiguous: map[string]bool{"acme-corp": true},
			want:                nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ambiguous := tc.crossShardAmbiguous
			if ambiguous == nil {
				ambiguous = noneAmbiguous
			}
			got := sweepableBoards(tc.stats, tc.hasGrace, tc.fullCatalog, tc.fullBoardListing, ambiguous)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sweepableBoards() = %v, want %v", got, tc.want)
			}
		})
	}
}

// HYDRATION_RETRY_DAYS widens the window during which a body-less row is re-offered for
// detail hydration, so an operator can repair a backlog the ordinary two-week window has
// already aged past (freehire#1866). Unset means the default; a value that is not a positive
// number is a config error, not a silent fallback — a typo on a repair run would otherwise
// look like a run that repaired nothing.
func TestHydrationRetryWindowFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"unset keeps the default", "", pipeline.HydrationRetryWindow, false},
		{"widened for a repair run", "365", 365 * 24 * time.Hour, false},
		{"narrowed", "1", 24 * time.Hour, false},
		{"not a number", "two weeks", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-5", 0, true},
	}
	for _, tc := range cases {
		got, err := hydrationRetryWindowFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: hydrationRetryWindowFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: hydrationRetryWindowFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// INGEST_REFETCH_ALL empties the seen-set so a crawl re-writes the provider's stored rows —
// the only way an adapter fix reaches postings ingested before it, since a re-listed posting
// otherwise takes the content-less refresh path. Anything but the two accepted spellings is a
// config error rather than a silent false, for the same reason HYDRATION_RETRY_DAYS is.
func TestRefetchAllFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    bool
		wantErr bool
	}{
		{"unset is an ordinary crawl", "", false, false},
		{"one", "1", true, false},
		{"true", "true", true, false},
		{"zero is not a spelling of off", "0", false, true},
		{"typo", "yes", false, true},
	}
	for _, tc := range cases {
		got, err := refetchAllFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: refetchAllFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: refetchAllFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}
