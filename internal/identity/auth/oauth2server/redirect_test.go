package oauth2server_test

import (
	"testing"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

func TestRedirectURIAllowed(t *testing.T) {
	registered := []string{
		"http://127.0.0.1:51739/callback",
		"https://app.example.com/oauth/callback",
	}
	cases := []struct {
		name      string
		candidate string
		want      bool
	}{
		{"exact match", "https://app.example.com/oauth/callback", true},
		{"loopback, different port", "http://127.0.0.1:60000/callback", true},
		{"loopback, different path", "http://127.0.0.1:51739/other", false},
		{"ipv6 loopback form not registered", "http://[::1]:51739/callback", false},
		{"unregistered host", "https://evil.example.com/callback", false},
		{"query string appended", "https://app.example.com/oauth/callback?x=1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oauth2server.RedirectURIAllowed(registered, tc.candidate); got != tc.want {
				t.Errorf("RedirectURIAllowed(%v, %q) = %v, want %v", registered, tc.candidate, got, tc.want)
			}
		})
	}
}
