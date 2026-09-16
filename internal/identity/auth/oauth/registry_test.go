package oauth

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/config"
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

func TestNewRegistry_AppleRequiresFullCredentialSet(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"apple": {ClientID: "me.freehire.web", TeamID: "team", KeyID: "key"}, // missing PrivateKey -> disabled
	})
	if names(reg)["apple"] {
		t.Error("apple enabled; want disabled (missing private key)")
	}
}

func TestNewRegistry_AppleEnabledWithFullCredentialSet(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"apple": {ClientID: "me.freehire.web", TeamID: "team", KeyID: "key", PrivateKey: "pem"},
	})
	if !names(reg)["apple"] {
		t.Error("apple missing; want enabled")
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

// A provider console registers ONE callback URL per application, so sign-in and
// re-authentication cannot each have their own. They used to: `ProviderV2` asked the
// provider to return to `/api/v2/...` while only `/api/v1/...` was ever registered,
// which GitHub refuses outright ("The redirect_uri is not associated with this
// application") — and which nobody noticed, because the buttons that start that flow
// only appeared after the server had already refused an action. Google happened to have
// both listed; GitHub, LinkedIn and Apple did not.
//
// Keeping the two in step is not something a comment can enforce, so it is asserted.
func TestRegistry_BothFlowsRedirectToTheSameRegisteredCallback(t *testing.T) {
	reg := NewRegistry(map[string]config.OAuthCredentials{
		"google": {ClientID: "id", ClientSecret: "secret"},
		"github": {ClientID: "id", ClientSecret: "secret"},
	})

	for _, name := range []string{"google", "github"} {
		signIn, ok := reg.Provider(name, "https://freehire.me")
		if !ok {
			t.Fatalf("%s not enabled for sign-in", name)
		}
		reauth, ok := reg.ProviderV2(name, "https://freehire.me")
		if !ok {
			t.Fatalf("%s not enabled for re-authentication", name)
		}
		want := "freehire.me%2Fapi%2Fv1%2Fauth%2Foauth%2F" + name + "%2Fcallback"
		if got := reauth.AuthCodeURL("s"); !strings.Contains(got, want) {
			t.Errorf("%s re-authentication redirect is not the registered callback:\n got %q\nwant substring %q", name, got, want)
		}
		if signIn.AuthCodeURL("s") != reauth.AuthCodeURL("s") {
			t.Errorf("%s: sign-in and re-authentication disagree about the callback:\n sign-in %q\n reauth  %q",
				name, signIn.AuthCodeURL("s"), reauth.AuthCodeURL("s"))
		}
	}
}
