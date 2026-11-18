package oauth2server_test

import (
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

func TestCanonicalResource(t *testing.T) {
	const origin = "https://freehire.me"
	const mixed, account = "/api/v1/mcp", "/api/v1/mcp/account"
	cases := []struct {
		name   string
		origin string // defaults to origin
		raw    []string
		want   string
		err    bool
	}{
		{"absent", "", nil, "", false},
		{"empty value is absent", "", []string{""}, "", false},
		{"mixed endpoint", "", []string{origin + mixed}, mixed, false},
		{"the old account endpoint binds the one resource", "", []string{origin + account}, mixed, false},
		{"trailing slash is normalised", "", []string{origin + mixed + "/"}, mixed, false},
		{"scheme and host are case-insensitive", "", []string{"HTTPS://FreeHire.ME" + mixed}, mixed, false},
		{"default https port is ignored", "", []string{"https://freehire.me:443" + mixed}, mixed, false},
		{"default port ignored on the configured origin too", "https://freehire.me:443", []string{origin + mixed}, mixed, false},
		{"http default port", "http://localhost", []string{"http://localhost:80" + mixed}, mixed, false},
		{"explicit non-default port must match", "http://localhost:8080", []string{"http://localhost:8080" + mixed}, mixed, false},
		{"non-default port mismatch", "", []string{"https://freehire.me:8443" + mixed}, "", true},
		{"repeated same value", "", []string{origin + mixed, origin + mixed + "/"}, mixed, false},
		{"old and new spellings are one resource", "", []string{origin + mixed, origin + account}, mixed, false},
		{"foreign host", "", []string{"https://evil.example" + mixed}, "", true},
		{"scheme downgrade", "", []string{"http://freehire.me" + mixed}, "", true},
		{"other path", "", []string{origin + "/api/v1/jobs"}, "", true},
		{"path case is exact", "", []string{origin + "/API/v1/mcp"}, "", true},
		{"dot segment", "", []string{origin + mixed + "/."}, "", true},
		{"double trailing slash", "", []string{origin + mixed + "//"}, "", true},
		{"query appended", "", []string{origin + mixed + "?x=1"}, "", true},
		{"bare question mark", "", []string{origin + mixed + "?"}, "", true},
		{"fragment appended", "", []string{origin + mixed + "#f"}, "", true},
		{"bare fragment marker", "", []string{origin + mixed + "#"}, "", true},
		{"userinfo", "", []string{"https://u@freehire.me" + mixed}, "", true},
		{"relative", "", []string{mixed}, "", true},
		{"no scheme", "", []string{"freehire.me" + mixed}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.origin
			if o == "" {
				o = origin
			}
			got, err := oauth2server.CanonicalResource(o, tc.raw)
			if tc.err {
				if !errors.Is(err, oauth2server.ErrInvalidTarget) {
					t.Fatalf("err = %v (got %q), want ErrInvalidTarget", err, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("CanonicalResource = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
