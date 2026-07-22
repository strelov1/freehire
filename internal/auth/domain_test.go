package auth

import "testing"

func TestCookieDomainForHost(t *testing.T) {
	domains := []string{"freehire.dev", "freehire.me"}
	cases := []struct {
		host string
		want string
	}{
		{"freehire.dev", ".freehire.dev"},       // apex
		{"apply.freehire.dev", ".freehire.dev"}, // subdomain shares
		{"freehire.me", ".freehire.me"},         // second domain
		{"agent.freehire.me", ".freehire.me"},   // its subdomain
		{"freehire.me:8080", ".freehire.me"},    // :port ignored
		{"localhost", ""},                       // dev -> host-only
		{"evil.com", ""},                        // unknown -> host-only
		{"notfreehire.dev", ""},                 // suffix must be dot-bounded
		{"freehire.dev.evil.com", ""},           // not our domain
	}
	for _, tc := range cases {
		if got := CookieDomainForHost(tc.host, domains); got != tc.want {
			t.Errorf("CookieDomainForHost(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

// With no configured domains (dev) every host is host-only.
func TestCookieDomainForHost_NoDomains(t *testing.T) {
	if got := CookieDomainForHost("freehire.me", nil); got != "" {
		t.Errorf("got %q, want empty when no domains configured", got)
	}
}
