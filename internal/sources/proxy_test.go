package sources

import (
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
