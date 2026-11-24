package mobileauth

import (
	"errors"
	"testing"
)

func TestChallengeRFC7636Vector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := Challenge(verifier); got != want {
		t.Fatalf("Challenge()=%q want %q", got, want)
	}
}

func TestVerifierAndChallengeBounds(t *testing.T) {
	valid := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"
	if len(valid) != 43 {
		t.Fatal("fixture")
	}
	if err := ValidateVerifier(valid); err != nil {
		t.Fatalf("valid verifier: %v", err)
	}
	for _, bad := range []string{valid[:42], valid + valid + valid, "abc=def"} {
		if ValidateVerifier(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if ValidateChallenge(Challenge(valid), "S256") != nil {
		t.Fatal("valid challenge rejected")
	}
	if ValidateChallenge(Challenge(valid), "plain") == nil {
		t.Fatal("plain accepted")
	}
}

// Linking is checked against the browser session cookie, which neither a native browser
// flow nor the native Apple exchange presents, so both refuse the purpose before touching
// storage (a nil pool proves nothing was written).
func TestLinkPurposeIsWebOnly(t *testing.T) {
	store := NewStore(nil)
	binding := &Binding{UserID: 1, TokenVersion: 1, SessionHash: make([]byte, 32)}
	challenge := Challenge("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ")
	for _, platform := range []string{"ios", "android"} {
		if _, _, err := store.CreateBrowserAttempt(t.Context(), "github", platform, platform, "link", challenge, binding); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s link attempt: err=%v, want ErrInvalid", platform, err)
		}
	}
	if _, err := store.CreateAppleAttempt(t.Context(), challenge, "link", []string{"aud"}, binding); !errors.Is(err, ErrInvalid) {
		t.Errorf("Apple link attempt: err=%v, want ErrInvalid", err)
	}
}

func TestLinkPurposeRequiresSessionBinding(t *testing.T) {
	if err := validatePurpose("link", nil); !errors.Is(err, ErrSession) {
		t.Fatalf("unbound link: err=%v, want ErrSession", err)
	}
	if err := validatePurpose("link", &Binding{UserID: 1, TokenVersion: 1, SessionHash: make([]byte, 32)}); err != nil {
		t.Fatalf("bound link: %v", err)
	}
}
