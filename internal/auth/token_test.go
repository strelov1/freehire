package auth

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

func TestIssuer_IssuesDistinctSessionTokens(t *testing.T) {
	iss := NewIssuer("test-secret", time.Hour)
	first, err := iss.Issue(42, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := iss.Issue(42, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two sessions received the same token")
	}
	firstFingerprint, err := iss.SessionFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := iss.SessionFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstFingerprint) == string(secondFingerprint) {
		t.Fatal("two sessions received the same fingerprint")
	}
}

func TestIssuer_LegacySessionCannotBindRecentAuth(t *testing.T) {
	iss := NewIssuer("test-secret", time.Hour)
	now := time.Now()
	legacy := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(42, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		TokenVersion: func() *int32 { v := int32(1); return &v }(),
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, legacy).SignedString(iss.secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = iss.Parse(raw); err != nil {
		t.Fatalf("legacy session lost rollout compatibility: %v", err)
	}
	if _, err = iss.SessionFingerprint(raw); !errors.Is(err, ErrNoSessionID) {
		t.Fatalf("legacy session fingerprint err=%v", err)
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
