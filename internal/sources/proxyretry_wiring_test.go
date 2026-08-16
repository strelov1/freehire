package sources

import (
	"testing"
)

// The two allowlists must stay disjoint: a provider in both would be rewired twice, and the
// wholly-proxied build would win by ordering rather than by decision — silently moving a whole
// crawl onto the shared proxy IP, which is the thing the refusal-retry list exists to avoid.
func TestRefusalRetryAndProxiedAllowlistsAreDisjoint(t *testing.T) {
	for name := range refusalRetryProviders {
		if _, both := proxiedProviders[name]; both {
			t.Errorf("%q is in both proxiedProviders and refusalRetryProviders; pick one — "+
				"wholly proxied (the direct IP is blocked) or refusal-retry (the direct IP works)", name)
		}
	}
}

// Every allowlisted name must be a real provider key, or the entry is inert and the platform
// keeps failing while the file claims it is handled.
func TestRefusalRetryProvidersAreRegistered(t *testing.T) {
	registry := Taxonomy()
	for name := range refusalRetryProviders {
		if _, ok := registry[name]; !ok {
			t.Errorf("refusalRetryProviders has %q, which is not a registered provider", name)
		}
	}
}

// ApplyProxyEgress must rewire the refusal-retry providers too. Without this the allowlist is
// documentation: the adapters keep the plain client and never reach the proxy at all.
func TestApplyProxyEgressRewiresRefusalRetryProviders(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")
	registry := All(NewClient())
	before := registry["teamtailor"]

	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	if registry["teamtailor"] == before {
		t.Error("teamtailor was not rewired; it still crawls on the client it was built with")
	}
	if got := registry["teamtailor"].Provider(); got != "teamtailor" {
		t.Errorf("rewired provider = %q, want teamtailor", got)
	}
}

// With no proxy configured the wiring must be a no-op, so local and dev runs behave exactly as
// they did — direct, with a 403 still final.
func TestApplyProxyEgressWithoutProxyLeavesRefusalRetryProvidersAlone(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "")
	registry := All(NewClient())
	before := registry["workable"]

	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	if registry["workable"] != before {
		t.Error("workable was rewired with no proxy configured")
	}
}
