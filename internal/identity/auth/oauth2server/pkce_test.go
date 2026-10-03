package oauth2server_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
)

func challengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestVerifyPKCE(t *testing.T) {
	verifier := "a-high-entropy-verifier-the-client-generated-1234567890"
	cases := []struct {
		name      string
		challenge string
		verifier  string
		want      bool
	}{
		{"matching S256 pair", challengeFor(verifier), verifier, true},
		{"wrong verifier", challengeFor(verifier), "something-else", false},
		{"empty verifier", challengeFor(verifier), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oauth2server.VerifyPKCE(tc.challenge, tc.verifier); got != tc.want {
				t.Errorf("VerifyPKCE(%q, %q) = %v, want %v", tc.challenge, tc.verifier, got, tc.want)
			}
		})
	}
}
