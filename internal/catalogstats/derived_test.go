package catalogstats

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

// The figure is labelled "ATS platforms" on /open, so it must count ATS platforms: the
// multi-tenant systems addressed by a board id, each serving many companies. The
// registry also holds aggregators (third-party feeds republishing many companies) and
// single-company career feeds, and counting those under this label is what the
// hardcoded frontend constant used to do.
func TestATSPlatformsCountsOnlyBoardKeyedNonAggregators(t *testing.T) {
	got := ATSPlatforms()

	registry := sources.Taxonomy()
	if got <= 0 {
		t.Fatalf("ATSPlatforms = %d, want a positive count", got)
	}
	if got >= len(registry) {
		t.Errorf("ATSPlatforms = %d against a %d-adapter registry — aggregators and "+
			"single-company feeds are being counted as ATS platforms", got, len(registry))
	}

	// Named adapters, so a predicate that drifts fails with a reason rather than a
	// number. greenhouse is a board-keyed ATS; adzuna is board-keyed but aggregates
	// other companies' postings; amazon is one company's own careers feed.
	boardKeyed := sources.BoardKeyedProviders(registry)
	aggregators := sources.AggregatorProviders(registry)

	for _, tc := range []struct {
		provider string
		wantATS  bool
		because  string
	}{
		{"greenhouse", true, "a multi-tenant ATS addressed by a board id"},
		{"adzuna", false, "an aggregator republishing many companies' postings"},
		{"amazon", false, "a single company's own careers feed"},
	} {
		isATS := slices.Contains(boardKeyed, tc.provider) && !slices.Contains(aggregators, tc.provider)
		if isATS != tc.wantATS {
			t.Errorf("%s counted as an ATS platform = %v, want %v — it is %s",
				tc.provider, isATS, tc.wantATS, tc.because)
		}
	}
}

// The /open strip leads with total reach, so the snapshot carries it alongside the
// narrower ATS figure. Both are derived from the same registry, and the wider one must
// actually be wider — if they ever coincide, one of the two predicates is wrong.
func TestSourcesCountsEveryRegisteredAdapter(t *testing.T) {
	got := Sources()

	if want := len(sources.Taxonomy()); got != want {
		t.Errorf("Sources = %d, want %d — every registered adapter counts, whatever kind it is", got, want)
	}
	if ats := ATSPlatforms(); got <= ats {
		t.Errorf("Sources = %d is not wider than ATSPlatforms = %d — aggregators and "+
			"single-company feeds are sources too, so the totals cannot coincide", got, ats)
	}
}

// Nothing about the count may live in a literal: adding an adapter must move it.
func TestATSPlatformsIsDerivedFromTheRegistry(t *testing.T) {
	registry := sources.Taxonomy()
	boardKeyed := sources.BoardKeyedProviders(registry)
	aggregators := sources.AggregatorProviders(registry)

	want := 0
	for _, p := range boardKeyed {
		if !slices.Contains(aggregators, p) {
			want++
		}
	}

	if got := ATSPlatforms(); got != want {
		t.Errorf("ATSPlatforms = %d, want %d — the count must fall out of the registry, not a constant", got, want)
	}
}
