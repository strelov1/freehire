package sources

import "testing"

// echojobs is the reason the tier exists; if it ever drops out of the map the provider goes
// back to a plain GET and to reporting an empty success (freehire#2588).
func TestBrowserTierCarriesEchojobs(t *testing.T) {
	if _, ok := browserProviders["echojobs"]; !ok {
		t.Error("echojobs is not in browserProviders")
	}
}

// The browser tier and the proxy tiers must be DISJOINT. Both rewire the same registry entry
// and cmd/ingest applies them in sequence, so a provider in both would silently get whichever
// ran last — a wiring bug with no symptom except a provider that quietly stopped using the
// transport somebody chose for it.
//
// A browser-tier provider does not need proxiedProviders anyway: ApplyBrowserEgress builds
// its session over SOURCES_PROXY_URL itself, so the browser IS the proxied transport.
//
// The HOSTED tier is the documented exception and is checked separately (see
// TestHostedTierDeliberatelyOverridesTheFingerprintTierForGulftalent): it may take a provider
// off another tier, because for its two providers every other transport is measured as
// refused. A rule with a stated exception beats a rule quietly broken.
func TestBrowserTierAndProxyTierAreDisjoint(t *testing.T) {
	for name := range browserProviders {
		if _, ok := proxiedProviders[name]; ok {
			t.Errorf("%s is in both browserProviders and proxiedProviders; the later rewire wins silently", name)
		}
		if _, ok := proxiedFingerprintProviders[name]; ok {
			t.Errorf("%s is in both browserProviders and proxiedFingerprintProviders", name)
		}
		if _, ok := refusalRetryProviders[name]; ok {
			t.Errorf("%s is in both browserProviders and refusalRetryProviders", name)
		}
	}
}

// With no proxy configured the tier is off and nothing is rewired: a browser alone cannot
// pass, so launching one would spend seconds to fail.
func TestApplyBrowserEgressIsANoOpWithoutAProxy(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "")
	registry := map[string]Source{"echojobs": NewEchoJobs(NewClient())}
	before := registry["echojobs"]

	closeBrowser, err := ApplyBrowserEgress(registry)
	if err != nil {
		t.Fatalf("ApplyBrowserEgress: %v", err)
	}
	defer closeBrowser()

	if registry["echojobs"] != before {
		t.Error("echojobs was rewired with no proxy configured")
	}
}

// With a proxy the provider is rewired — and still no browser has started. Chrome launches on
// the first fetch, not at wiring time, so an ingest run for any OTHER provider never pays for
// one. This test would take seconds and need a browser installed if that were not true.
func TestApplyBrowserEgressRewiresLazily(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.invalid:8080")
	registry := map[string]Source{"echojobs": NewEchoJobs(NewClient())}
	before := registry["echojobs"]

	closeBrowser, err := ApplyBrowserEgress(registry)
	if err != nil {
		t.Fatalf("ApplyBrowserEgress: %v", err)
	}
	defer closeBrowser()

	if registry["echojobs"] == before {
		t.Error("echojobs was not rewired onto the browser tier")
	}
}

// A set-but-unparseable proxy fails at wiring rather than leaving the provider on a transport
// that cannot work — the same fail-fast contract ApplyProxyEgress has, and for the same
// reason: a silent fallback here is indistinguishable from a crawl that found nothing.
func TestApplyBrowserEgressRejectsAnUnparseableProxy(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "://not a url")
	registry := map[string]Source{"echojobs": NewEchoJobs(NewClient())}
	if _, err := ApplyBrowserEgress(registry); err == nil {
		t.Error("want an error for an unparseable SOURCES_PROXY_URL, got nil")
	}
}

// A registry that does not carry a browser-tier provider is left completely alone, so an
// ingest run for any other provider costs nothing.
func TestApplyBrowserEgressIgnoresAnUnrelatedRegistry(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.invalid:8080")
	registry := map[string]Source{"greenhouse": NewGreenhouse(NewClient())}
	before := registry["greenhouse"]

	closeBrowser, err := ApplyBrowserEgress(registry)
	if err != nil {
		t.Fatalf("ApplyBrowserEgress: %v", err)
	}
	defer closeBrowser()

	if registry["greenhouse"] != before {
		t.Error("an unrelated provider was rewired")
	}
}
