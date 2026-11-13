package oauth2server_test

import (
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

func TestCanonicalResource(t *testing.T) {
	const origin = "https://freehire.me"
	cases := []struct {
		name string
		raw  []string
		want string
		err  bool
	}{
		{"absent", nil, "", false},
		{"empty value is absent", []string{""}, "", false},
		{"mixed endpoint", []string{origin + "/api/v1/mcp"}, origin + "/api/v1/mcp", false},
		{"account endpoint", []string{origin + "/api/v1/mcp/account"}, origin + "/api/v1/mcp/account", false},
		{"trailing slash is normalised", []string{origin + "/api/v1/mcp/"}, origin + "/api/v1/mcp", false},
		{"repeated same value", []string{origin + "/api/v1/mcp", origin + "/api/v1/mcp/"}, origin + "/api/v1/mcp", false},
		{"two distinct values", []string{origin + "/api/v1/mcp", origin + "/api/v1/mcp/account"}, "", true},
		{"foreign host", []string{"https://evil.example/api/v1/mcp"}, "", true},
		{"other path", []string{origin + "/api/v1/jobs"}, "", true},
		{"query appended", []string{origin + "/api/v1/mcp?x=1"}, "", true},
		{"fragment appended", []string{origin + "/api/v1/mcp#f"}, "", true},
		{"different case", []string{"https://FREEHIRE.me/api/v1/mcp"}, "", true},
		{"relative", []string{"/api/v1/mcp"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := oauth2server.CanonicalResource(origin, tc.raw)
			if tc.err {
				if !errors.Is(err, oauth2server.ErrInvalidTarget) {
					t.Fatalf("err = %v, want ErrInvalidTarget", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("CanonicalResource = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestResourceAllows(t *testing.T) {
	const a, b = "https://freehire.me/api/v1/mcp", "https://freehire.me/api/v1/mcp/account"
	if !oauth2server.ResourceAllows("", a) || !oauth2server.ResourceAllows("", b) {
		t.Error("an unbound grant must be valid on every endpoint")
	}
	if !oauth2server.ResourceAllows(a, a) {
		t.Error("a grant is valid at its own resource")
	}
	if oauth2server.ResourceAllows(a, b) || oauth2server.ResourceAllows(b, a) {
		t.Error("a bound grant must not cross endpoints")
	}
}
