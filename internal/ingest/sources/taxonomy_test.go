package sources

import (
	"slices"
	"testing"
)

// keyedProviders are the adapters whose crawl needs an environment credential. They are the
// ones the two registries disagree about, so both tests below iterate exactly this set.
// keyedProviders lists reed, usajobs, francetravail, arc and every whatjobs market provider — built from
// whatjobsMarkets so a new market added there is covered here automatically.
var keyedProviders = func() []string {
	out := []string{"reed", "usajobs", "francetravail", "arc"}
	for _, m := range whatjobsMarkets {
		out = append(out, m.provider)
	}
	return out
}()

// clearCrawlCredentials unsets every keyed adapter's variable for the duration of the test,
// which is the state of a reindex host, a status-serving API host and a contributor's laptop.
func clearCrawlCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("USAJOBS_API_KEY", "")
	t.Setenv("REED_API_KEY", "")
	t.Setenv("FRANCE_TRAVAIL_CLIENT_ID", "")
	t.Setenv("FRANCE_TRAVAIL_CLIENT_SECRET", "")
	t.Setenv("WHATJOBS_PUBLISHER_IDS", "")
	t.Setenv(arcSessionEnv, "")
}

// The taxonomy registry answers "what kind of source is this provider" — a compile-time fact
// about the adapter type. It must not vary with crawl credentials: cmd/reindex's aggregator
// suppression, cmd/ghost-crosscheck, the status page's provider kind and the generated source
// facet all read it, and a keyless host has to classify exactly as a configured one does.
// Before this was true, a keyless reindex classified whatjobs — a CPC reseller of first-party
// ATS postings — as an ATS itself, so none of its copies were ever suppressed.
func TestTaxonomyIsTotalWithoutCrawlCredentials(t *testing.T) {
	clearCrawlCredentials(t)

	registry := Taxonomy()
	aggregators := AggregatorProviders(registry)
	facet := FilterableProviders()

	for _, p := range keyedProviders {
		if _, ok := registry[p]; !ok {
			t.Errorf("Taxonomy() is missing %q — every consumer would read it as KindOther", p)
			continue
		}
		if got := ProviderKind(registry, p); got != KindAggregator {
			t.Errorf("ProviderKind(%q) = %q, want %q", p, got, KindAggregator)
		}
		if !slices.Contains(aggregators, p) {
			t.Errorf("AggregatorProviders() is missing %q — its copies of an ATS posting would never be suppressed", p)
		}
		if !slices.Contains(facet, p) {
			t.Errorf("FilterableProviders() is missing %q", p)
		}
	}
}

// The other half of the split: the crawl registry still gates on the credential, so a board
// file naming an unconfigured provider fails config validation before any request is made
// rather than starting crawls that fail per board and cool them.
func TestCrawlRegistryOmitsUnconfiguredProviders(t *testing.T) {
	clearCrawlCredentials(t)

	registry := All(NewClient())
	for _, p := range keyedProviders {
		if _, ok := registry[p]; ok {
			t.Errorf("All(client) registered %q with no credential — ingest would crawl boards that cannot authenticate", p)
		}
	}
}

func TestTaxonomyCacheOwnership(t *testing.T) {
	first := Taxonomy()
	delete(first, "hh")
	if _, ok := Taxonomy()["hh"]; !ok {
		t.Fatal("caller changed cached taxonomy")
	}
	providers := TaxonomyAggregatorProviders()
	want := AggregatorProviders(Taxonomy())
	if !slices.Equal(providers, want) {
		t.Fatal("cached aggregator classification differs")
	}
	providers[0] = "changed"
	if !slices.Equal(TaxonomyAggregatorProviders(), want) {
		t.Fatal("caller changed cached aggregators")
	}
}

func TestTaxonomyReadAllocations(t *testing.T) {
	Taxonomy()
	TaxonomyAggregatorProviders()
	if got := testing.AllocsPerRun(100, func() { Taxonomy() }); got > 10 {
		t.Fatalf("taxonomy read allocated %g times; full registry must not be rebuilt", got)
	}
	if got := testing.AllocsPerRun(100, func() { TaxonomyAggregatorProviders() }); got > 1 {
		t.Fatalf("aggregator read allocated %g times", got)
	}
}

func BenchmarkTaxonomy(b *testing.B) {
	Taxonomy()
	b.ReportAllocs()
	for b.Loop() {
		Taxonomy()
	}
}

func BenchmarkTaxonomyAggregatorProviders(b *testing.B) {
	TaxonomyAggregatorProviders()
	b.ReportAllocs()
	for b.Loop() {
		TaxonomyAggregatorProviders()
	}
}

func BenchmarkTaxonomyUncached(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		All(nil)
	}
}
