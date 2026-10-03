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
