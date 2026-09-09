package sources

import (
	"errors"
	"net/http"
	"testing"
)

// The hosted client's errors must be the same type the plain client produces, for the reason
// the browser client's are: detailUnreadable and isRateLimited match on *StatusError, so a
// hosted 404 that arrived as a bare error would stop meaning "this posting is gone".
func TestFirecrawlStatusErrorMatchesThePlainClient(t *testing.T) {
	if err := firecrawlStatusError("https://example.test/x", http.StatusOK); err != nil {
		t.Errorf("200 produced an error: %v", err)
	}
	err := firecrawlStatusError("https://example.test/x", http.StatusNotFound)
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error is %T, want *StatusError", err)
	}
	if se.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want 404", se.Code)
	}
}

// Both target adapters are unchanged by this tier, which only holds if the client satisfies
// exactly what they already declare.
func TestFirecrawlClientSatisfiesBothAdaptersTransports(t *testing.T) {
	var c any = &firecrawlClient{}
	if _, ok := c.(baytHTTP); !ok {
		t.Error("firecrawlClient does not satisfy baytHTTP")
	}
	if _, ok := c.(gulftalentHTTP); !ok {
		t.Error("firecrawlClient does not satisfy gulftalentHTTP")
	}
}

func TestHostedTierCarriesTheTwoUnreachableProviders(t *testing.T) {
	for _, name := range []string{"bayt", "gulftalent", "wantapply", "hh"} {
		if _, ok := firecrawlProviders[name]; !ok {
			t.Errorf("%s is not in firecrawlProviders", name)
		}
	}
}

// hh is the mixed-tier case with no proxy involved at all: only detail hydration is hosted,
// and listing keeps a fresh direct client REGARDLESS of SOURCES_PROXY_URL, because hh's own
// detail pages are what's blocked (by DDoS-Guard, through the proxy) while listing measurably
// isn't. Unlike wantapply, which reuses ApplyFirecrawlEgress's shared `direct` transport for
// its non-hosted pages, hh must NOT reuse it — that value becomes the proxied client whenever
// SOURCES_PROXY_URL is set for any OTHER provider, which would put hh's listing right back on
// the proxy this change exists to stop using.
func TestApplyFirecrawlEgressRewiresHHDetailWithAKey(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "test-key")

	registry := map[string]Source{"hh": NewHH(NewClient())}
	before := registry["hh"]

	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["hh"] == before {
		t.Error("hh was not rewired onto the hosted detail transport")
	}
}

func TestHHKeepsItsCurrentTransportWithoutAKey(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "")

	registry := map[string]Source{"hh": NewHH(NewClient())}
	before := registry["hh"]

	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["hh"] != before {
		t.Error("hh was rewired with no API key configured")
	}
}

// Without a key nothing is rewired and no client is built, so merging this change cannot
// spend anything.
func TestApplyFirecrawlEgressIsANoOpWithoutAKey(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "")
	registry := map[string]Source{"bayt": NewBayt(NewClient()), "gulftalent": NewGulfTalent(NewClient())}
	before := map[string]Source{"bayt": registry["bayt"], "gulftalent": registry["gulftalent"]}

	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	for name, was := range before {
		if registry[name] != was {
			t.Errorf("%s was rewired with no API key configured", name)
		}
	}
}

func TestApplyFirecrawlEgressRewiresBothWithAKey(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "test-key")
	registry := map[string]Source{"bayt": NewBayt(NewClient()), "gulftalent": NewGulfTalent(NewClient())}
	before := map[string]Source{"bayt": registry["bayt"], "gulftalent": registry["gulftalent"]}

	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	for name, was := range before {
		if registry[name] == was {
			t.Errorf("%s was not rewired onto the hosted tier", name)
		}
	}
}

// gulftalent sits in proxiedFingerprintProviders too, and both tiers rewire the same registry
// entry. The override is WANTED here — the fingerprint transport is measured as refused and
// the hosted one as served — so the order is pinned rather than left to whichever ran last.
func TestHostedTierDeliberatelyOverridesTheFingerprintTierForGulftalent(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.invalid:8080")
	t.Setenv("FIRECRAWL_API_KEY", "test-key")

	registry := map[string]Source{"gulftalent": NewGulfTalent(NewClient())}
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	viaFingerprint := registry["gulftalent"]
	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["gulftalent"] == viaFingerprint {
		t.Error("gulftalent kept the fingerprint transport; the hosted tier must run last and win")
	}
}

// And the fallback: with no key it keeps exactly the transport it has today, so an
// unconfigured deployment is bit-for-bit unchanged.
func TestGulftalentKeepsTheFingerprintTierWithoutAKey(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.invalid:8080")
	t.Setenv("FIRECRAWL_API_KEY", "")

	registry := map[string]Source{"gulftalent": NewGulfTalent(NewClient())}
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	viaFingerprint := registry["gulftalent"]
	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["gulftalent"] != viaFingerprint {
		t.Error("gulftalent lost its fingerprint transport with no hosted key configured")
	}
}

func TestApplyFirecrawlEgressIgnoresAnUnrelatedRegistry(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "test-key")
	registry := map[string]Source{"greenhouse": NewGreenhouse(NewClient())}
	before := registry["greenhouse"]

	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["greenhouse"] != before {
		t.Error("an unrelated provider was rewired")
	}
}

// A set-but-unusable budget fails the run rather than falling back to a default. This is the
// one knob that decides how much money a night can cost, so a typo in it must not be absorbed.
func TestApplyFirecrawlEgressRejectsAnUnparseableBudget(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "test-key")
	t.Setenv("FIRECRAWL_MAX_PAGES_PER_RUN", "lots")
	registry := map[string]Source{"bayt": NewBayt(NewClient())}
	if err := ApplyFirecrawlEgress(registry); err == nil {
		t.Error("want an error for an unparseable page budget, got nil")
	}
}

// wantapply is the mixed-tier case: only its sitemap is hosted, so it must still be rewired
// (the enumeration changes) while keeping a free transport for the pages. Without a key it
// keeps whatever ApplyProxyEgress gave it, exactly as today.
func TestHostedTierRewiresWantapplyForItsSitemapOnly(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.invalid:8080")
	t.Setenv("FIRECRAWL_API_KEY", "test-key")

	registry := map[string]Source{"wantapply": NewWantapply(NewClient())}
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	viaProxy := registry["wantapply"]
	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["wantapply"] == viaProxy {
		t.Error("wantapply was not rewired onto the fuller .com enumeration")
	}
}

func TestWantapplyKeepsItsProxyTransportWithoutAKey(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.invalid:8080")
	t.Setenv("FIRECRAWL_API_KEY", "")

	registry := map[string]Source{"wantapply": NewWantapply(NewClient())}
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	viaProxy := registry["wantapply"]
	if err := ApplyFirecrawlEgress(registry); err != nil {
		t.Fatalf("ApplyFirecrawlEgress: %v", err)
	}
	if registry["wantapply"] != viaProxy {
		t.Error("wantapply lost its proxied transport with no hosted key configured")
	}
}
