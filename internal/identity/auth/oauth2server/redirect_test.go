package oauth2server_test

import (
	"strings"
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
		{"loopback, extra query", "http://127.0.0.1:5000/callback?next=evil", false},
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

func TestRedirectURIAllowedRegisteredQuery(t *testing.T) {
	registered := []string{"http://127.0.0.1/cb?a=1"}
	if oauth2server.RedirectURIAllowed(registered, "http://127.0.0.1:5000/cb?a=1") {
		t.Error("loopback match admitted a registered query with a different port")
	}
}

func TestValidateRedirectURI(t *testing.T) {
	cases := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"hosted https client", "https://claude.ai/api/mcp/auth_callback", false},
		{"loopback http ipv4", "http://127.0.0.1:51739/callback", false},
		{"loopback http localhost", "http://localhost:33418/callback", false},
		{"loopback http ipv6", "http://[::1]:8080/callback", false},
		{"private-use app scheme", "cursor://anysphere.cursor-retrieval/oauth/callback", false},
		{"plain http off loopback", "http://attacker.example.com/cb", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"data scheme", "data:text/html,hi", true},
		{"file scheme", "file:///etc/passwd", true},
		{"vbscript scheme", "vbscript:msgbox(1)", true},
		{"blob scheme", "blob:https://app.example/x", true},
		{"about scheme", "about:blank", true},
		{"empty fragment", "https://app.example/cb#", true},
		{"relative", "/callback", true},
		{"fragment", "https://app.example.com/cb#frag", true},
		{"https without host", "https:///cb", true},
		{"too long", "https://app.example.com/" + strings.Repeat("a", 2048), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := oauth2server.ValidateRedirectURI(tc.uri)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateRedirectURI(%q) error = %v, wantErr %v", tc.uri, err, tc.wantErr)
			}
		})
	}
}
