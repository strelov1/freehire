package handler

import "testing"

// Leaving SERVED_HOSTS unset narrows the OAuth redirect origin to the frontend origin's
// own host. That is right for a single-domain deployment and wrong for one that answers
// on several — where it silently breaks sign-in on every other domain, because the state
// cookie is set on the host the flow started from and the callback lands elsewhere. A
// configured COOKIE_DOMAIN is exactly the signal that more than one host is served, so
// that combination has to be called out at startup rather than discovered in production.
func TestNeedsExplicitServedHosts(t *testing.T) {
	cases := []struct {
		name          string
		served        []string
		cookieDomains []string
		want          bool
	}{
		{"multi-domain deployment with no allowlist", nil, []string{"freehire.dev", "freehire.me"}, true},
		{"single-domain dev deployment", nil, nil, false},
		{"allowlist configured", []string{"freehire.dev"}, []string{"freehire.dev"}, false},
	}
	for _, tc := range cases {
		if got := needsExplicitServedHosts(tc.served, tc.cookieDomains); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
