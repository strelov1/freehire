package oauth2server_test

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

func TestGenerateAccessToken(t *testing.T) {
	token, hash, err := oauth2server.GenerateAccessToken()
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if !strings.HasPrefix(token, "fhm_") {
		t.Errorf("token %q lacks the fhm_ prefix", token)
	}
	if hash != oauth2server.HashToken(token) {
		t.Errorf("hash does not match HashToken(token)")
	}
	token2, _, err := oauth2server.GenerateAccessToken()
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if token == token2 {
		t.Errorf("two calls produced the same token")
	}
}

func TestGenerateAuthorizationCode(t *testing.T) {
	code, hash, err := oauth2server.GenerateAuthorizationCode()
	if err != nil {
		t.Fatalf("GenerateAuthorizationCode: %v", err)
	}
	if !strings.HasPrefix(code, "fhc_") {
		t.Errorf("code %q lacks the fhc_ prefix", code)
	}
	if hash != oauth2server.HashToken(code) {
		t.Errorf("hash does not match HashToken(code)")
	}
}

// A minted access token has the shape the rate limiter checks for before spending a lookup;
// anything else — a code, an API key, a truncated or mangled token — does not.
func TestWellFormedAccessToken(t *testing.T) {
	token, _, err := oauth2server.GenerateAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := oauth2server.GenerateAuthorizationCode()
	if err != nil {
		t.Fatal(err)
	}
	if !oauth2server.WellFormedAccessToken(token) {
		t.Errorf("a minted token %q is not well formed", token)
	}
	for _, bad := range []string{"", code, "fhk_" + token[4:], token[:len(token)-1], token[:len(token)-1] + "!"} {
		if oauth2server.WellFormedAccessToken(bad) {
			t.Errorf("%q is well formed", bad)
		}
	}
}
