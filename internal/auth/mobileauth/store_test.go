package mobileauth

import "testing"

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
