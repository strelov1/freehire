package sources

import (
	"net/url"
	"strings"
	"testing"
)

// proxiedProbe is a provider we can safely rewire in tests without a live client.
const proxiedProbe = "eightfold"

func TestApplyProxyEgressNoopWhenUnset(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "")

	registry := All(NewClient())
	before := registry[proxiedProbe]

	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("unset proxy should be a no-op, got %v", err)
	}
	if registry[proxiedProbe] != before {
		t.Error("unset proxy must leave the proxied provider on the direct client")
	}
}

func TestApplyProxyEgressRewiresProxiedProviderOnly(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")

	registry := All(NewClient())
	proxiedBefore := map[string]Source{}
	for name := range proxiedProviders {
		proxiedBefore[name] = registry[name]
	}
	directBefore := registry["greenhouse"] // not in proxiedProviders

	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("valid proxy: %v", err)
	}
	for name, before := range proxiedBefore {
		if registry[name] == before {
			t.Errorf("proxied provider %q must be rewired onto the proxy client", name)
		}
	}
	if registry["greenhouse"] != directBefore {
		t.Error("a non-proxied provider must stay on the direct client")
	}
}

// hh's detail pages are captcha-walled through the proxy (DDoS-Guard) while working fine on the
// direct IP; detail hydration moved to the Scrapling tier instead (scraplingtier.go), so hh no
// longer belongs in proxiedProviders at all — not even for listing, which was measured working
// directly.
func TestApplyProxyEgressLeavesHHOnTheDirectClient(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")

	registry := All(NewClient())
	before := registry["hh"]

	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("valid proxy: %v", err)
	}
	if registry["hh"] != before {
		t.Error("hh must stay on the direct client; it is no longer a proxied provider")
	}
}

func TestApplyProxyEgressInvalidURLFailsWithoutLeakingCreds(t *testing.T) {
	// A control character makes url.Parse fail; the password must not surface in the error.
	t.Setenv("SOURCES_PROXY_URL", "http://user:sup3rsecret@proxy.example:8080/\x7f")

	err := ApplyProxyEgress(All(NewClient()))
	if err == nil {
		t.Fatal("expected a set-but-invalid proxy URL to error")
	}
	if strings.Contains(err.Error(), "sup3rsecret") {
		t.Errorf("error leaked the proxy password: %v", err)
	}
}

func TestRedactProxyStripsPassword(t *testing.T) {
	cases := []string{
		"http://user:sup3rsecret@proxy.example:8080",
		"http://user:sup3rsecret@proxy.example:8080/\x7f", // unparseable tail
	}
	for _, in := range cases {
		if got := redactProxy(in); strings.Contains(got, "sup3rsecret") {
			t.Errorf("redactProxy(%q) = %q, still contains the password", in, got)
		}
	}
}

// naukri's posting API refuses the one fixed proxy exit ("recaptcha required") and serves the
// rotating gateway, a fresh exit per request; every other proxied provider keeps the fixed
// exit, and without a rotating gateway naukri keeps it too.
func TestProxyForSendsOnlyTheRotatingProvidersThroughTheGateway(t *testing.T) {
	fixed, _ := url.Parse("http://user:pass@203.0.113.7:5987")
	rotating, _ := url.Parse("http://user-rotate:pass@p.example:80")
	if got := proxyFor("naukri", fixed, rotating); got != rotating {
		t.Errorf("naukri proxy = %v, want the rotating gateway", got)
	}
	if got := proxyFor("enlizt", fixed, rotating); got != fixed {
		t.Errorf("enlizt proxy = %v, want the fixed exit", got)
	}
	if got := proxyFor("naukri", fixed, nil); got != fixed {
		t.Errorf("naukri without a gateway = %v, want the fixed exit", got)
	}
}

func TestApplyProxyEgressRefusesAnUnparseableRotatingGateway(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "http://user:sup3rsecret@proxy.example:80/\x7f")
	err := ApplyProxyEgress(All(NewClient()))
	if err == nil || strings.Contains(err.Error(), "sup3rsecret") {
		t.Fatalf("err = %v, want a refusal that does not print the password", err)
	}
}
