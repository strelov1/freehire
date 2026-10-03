package handler

import "testing"

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
