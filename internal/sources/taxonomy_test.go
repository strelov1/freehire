package sources

import (
	"slices"
	"testing"
)

// keyedProviders are the adapters whose crawl needs an environment credential. They are the
// ones the two registries disagree about, so both tests below iterate exactly this set.
// keyedProviders lists reed, usajobs and every whatjobs market provider — built from
// whatjobsMarkets so a new market added there is covered here automatically.
var keyedProviders = func() []string {
	out := []string{"reed", "usajobs"}
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
	t.Setenv("WHATJOBS_PUBLISHER_IDS", "")
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
