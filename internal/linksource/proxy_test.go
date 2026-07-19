package linksource

import (
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

// clientOf returns the transport an adapter was built with, so a test can assert which
// egress (direct vs proxy) a provider is wired to.
func clientOf(s Source) Client {
	switch a := s.(type) {
	case habrCareer:
		return a.http
	case remoteYeah:
		return a.http
	case geekjob:
		return a.http
	case greenhouse:
		return a.http
	case ashby:
		return a.http
	case lever:
		return a.http
	case workable:
		return a.http
	case bairesdev:
		return a.http
	default:
		return nil
	}
}

// TestAllWithProxyEgressRoutesProxiedProvidersThroughProxy asserts the single-URL resolve
// path mirrors sources.ApplyProxyEgress: providers on the proxied allowlist (e.g.
// habr_career and geekjob, behind Qrator/a WAF that challenges the prod datacenter IP)
// egress through the proxy client, while every other adapter stays on the direct client.
func TestAllWithProxyEgressRoutesProxiedProvidersThroughProxy(t *testing.T) {
	direct := &fakeClient{}
	proxy := &fakeClient{}

	reg := AllWithProxyEgress(direct, proxy)

	sawProxied := false
	for _, s := range reg {
		want := Client(direct)
		if sources.IsProxied(s.Source()) {
			want = proxy
			sawProxied = true
		}
		if got := clientOf(s); got != want {
			t.Errorf("%s adapter wired to wrong client: got %p, want %p", s.Source(), got, want)
		}
	}
	if !sawProxied {
		t.Fatal("no proxied provider in the registry — the test would prove nothing")
	}
}

// TestAllWithProxyEgressNilProxyStaysDirect asserts a nil proxy (SOURCES_PROXY_URL unset)
// leaves every adapter on the direct client, exactly like All.
func TestAllWithProxyEgressNilProxyStaysDirect(t *testing.T) {
	direct := &fakeClient{}
	for _, s := range AllWithProxyEgress(direct, nil) {
		if got := clientOf(s); got != Client(direct) {
			t.Errorf("%s adapter not on direct client with nil proxy: got %p", s.Source(), got)
		}
	}
}
