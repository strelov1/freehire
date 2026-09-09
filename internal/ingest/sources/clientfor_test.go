package sources

import (
	"testing"
)

// A caller that builds its own client (cmd/harvest-boards probes a candidate board without
// going through the provider's adapter) must get the same egress policy ApplyProxyEgress
// gives the crawl. Without this the harvest talks to workable on exactly the direct IP the
// workable entry in refusalRetryProviders exists to keep it off.
func TestClientForGivesRefusalRetryProvidersTheProxyFallback(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")

	c, err := ClientFor("workable")
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if c.refusalClient == nil {
		t.Error("workable got a client with no refusal fallback; a 429 from the direct IP stays final")
	}
}

// A wholly-proxied provider's direct path never works, so its client must egress through the
// proxy outright rather than only after a refusal.
func TestClientForGivesProxiedProvidersTheProxiedClient(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")

	c, err := ClientFor("djinni")
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if c.refusalClient != nil {
		t.Error("djinni got a refusal-retry client; its direct IP is blocked, so there is nothing to retry from")
	}
}

// Everything else keeps the direct client: the proxy is a single shared address, and a
// provider with no measured problem must not be moved onto it.
func TestClientForLeavesUnlistedProvidersDirect(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")

	c, err := ClientFor("greenhouse")
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if c.refusalClient != nil {
		t.Error("greenhouse got a refusal-retry client without being listed for one")
	}
}

// With no proxy configured every provider stays direct, so a local or dev run behaves exactly
// as it did.
func TestClientForWithoutProxyIsDirect(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "")

	c, err := ClientFor("workable")
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if c.refusalClient != nil {
		t.Error("workable got a proxy fallback with no proxy configured")
	}
}

// A set-but-unparseable value must fail the caller rather than hand back the direct client:
// silently crawling the blocked path is the one outcome an operator who configured a proxy
// would never notice.
func TestClientForFailsOnUnparseableProxy(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "://not a url")

	if _, err := ClientFor("workable"); err == nil {
		t.Error("ClientFor accepted an unparseable SOURCES_PROXY_URL and fell back to the direct client")
	}
}
