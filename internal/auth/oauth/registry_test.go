package oauth

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/config"
)

func names(r *Registry) map[string]bool {
	m := make(map[string]bool)
	for _, n := range r.Names() {
		m[n] = true
	}
	return m
}

func TestNewRegistry_OnlyCompleteCredentialsEnable(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"google":   {ClientID: "id", ClientSecret: "secret"},
		"github":   {ClientID: "id"}, // missing secret -> disabled
		"linkedin": {},               // unset -> disabled
	})

	got := names(reg)
	if !got["google"] {
		t.Error("google missing; want enabled")
	}
	if got["github"] {
		t.Error("github enabled; want disabled (no secret)")
	}
	if got["linkedin"] {
		t.Error("linkedin enabled; want disabled (unset)")
	}
}

func TestNewRegistry_IgnoresUnknownProvider(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"myspace": {ClientID: "id", ClientSecret: "secret"},
	})
	if len(reg.Names()) != 0 {
		t.Errorf("registry names = %v, want empty", reg.Names())
	}
}

func TestRegistry_ProviderRedirectURLDerivesFromOrigin(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"google": {ClientID: "id", ClientSecret: "secret"},
	})

	// The redirect URL — and thus the serving domain — comes from the origin
	// passed at build time, so the same registry answers for either domain.
	for _, host := range []string{"freehire.dev", "freehire.me"} {
		p, ok := reg.Provider("google", "https://"+host)
		if !ok {
			t.Fatalf("google not enabled for host %q", host)
		}
		u := p.AuthCodeURL("s")
		want := host + "%2Fapi%2Fv1%2Fauth%2Foauth%2Fgoogle%2Fcallback"
		if !strings.Contains(u, want) {
			t.Errorf("AuthCodeURL %q missing redirect URL for %q (want substring %q)", u, host, want)
		}
	}
}

func TestRegistry_ProviderUnknownIsNotOK(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"google": {ClientID: "id", ClientSecret: "secret"},
	})
	if _, ok := reg.Provider("github", "https://freehire.me"); ok {
		t.Error("github reported enabled; want not ok")
	}
}
