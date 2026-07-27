package auth

import (
	"testing"
	"time"
)

func TestIssuer_IssueParseRoundTrip(t *testing.T) {
	iss := NewIssuer("test-secret", time.Hour)

	token, err := iss.Issue(42, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, _, err := iss.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got != 42 {
		t.Errorf("Parse returned user id %d, want 42", got)
	}
}

func TestIssuer_RejectsExpiredToken(t *testing.T) {
	iss := NewIssuer("test-secret", -time.Minute) // already expired on issue

	token, err := iss.Issue(42, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, _, err := iss.Parse(token); err == nil {
		t.Error("Parse should reject an expired token")
	}
}

func TestIssuer_RejectsWrongSignature(t *testing.T) {
	signed := NewIssuer("real-secret", time.Hour)
	other := NewIssuer("different-secret", time.Hour)

	token, err := signed.Issue(42, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, _, err := other.Parse(token); err == nil {
		t.Error("Parse should reject a token signed with a different secret")
	}
}
