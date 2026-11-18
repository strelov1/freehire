package handler

import (
	"net/url"
	"testing"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

// parseAuthorizeParams is pure and DB-free, so its validation rules are covered
// here directly; the full GET/POST flow (client lookup, redirect matching,
// session handling, code issuance) needs a real Postgres and lives in
// oauth_authorize_integration_test.go, mirroring how api_keys_test.go /
// api_keys_integration_test.go split.
func TestParseAuthorizeParams(t *testing.T) {
	cases := []struct {
		name                                                          string
		responseType, clientID, redirectURI, state, challenge, method string
		wantErr                                                       bool
	}{
		{"valid", "code", "client-1", "http://127.0.0.1:9999/cb", "xyz", "chal", "S256", false},
		{"wrong response_type", "token", "client-1", "http://127.0.0.1:9999/cb", "xyz", "chal", "S256", true},
		{"wrong challenge method", "code", "client-1", "http://127.0.0.1:9999/cb", "xyz", "chal", "plain", true},
		{"missing client_id", "code", "", "http://127.0.0.1:9999/cb", "xyz", "chal", "S256", true},
		{"missing redirect_uri", "code", "client-1", "", "xyz", "chal", "S256", true},
		{"missing code_challenge", "code", "client-1", "http://127.0.0.1:9999/cb", "xyz", "", "S256", true},
		{"missing state is allowed (optional)", "code", "client-1", "http://127.0.0.1:9999/cb", "", "chal", "S256", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAuthorizeParams(tc.responseType, tc.clientID, tc.redirectURI, tc.state, tc.challenge, tc.method)
			if tc.wantErr && err == nil {
				t.Errorf("parseAuthorizeParams(%+v) = nil, want error", tc)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("parseAuthorizeParams(%+v) = %v, want nil", tc, err)
			}
		})
	}
}

// The sign-in detour must hand authorize back a resource it accepts: a bare path there
// failed every signed-out connector sign-in with invalid_target.
func TestOAuthSignInURLKeepsAnAcceptableResource(t *testing.T) {
	const origin = "https://freehire.me"
	h := &authHandlers{frontendOrigin: origin}
	signIn, err := url.Parse(h.oauthSignInURL(authorizeParams{ClientID: "c", RedirectURI: "http://127.0.0.1:9999/cb", Resource: oauth2server.MCPResourcePath}))
	if err != nil {
		t.Fatal(err)
	}
	back, err := url.Parse(signIn.Query().Get("returnTo"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := oauth2server.CanonicalResource(origin, back.Query()["resource"])
	if err != nil || got != oauth2server.MCPResourcePath {
		t.Fatalf("CanonicalResource(%v) = %q, %v; want %q", back.Query()["resource"], got, err, oauth2server.MCPResourcePath)
	}
}
