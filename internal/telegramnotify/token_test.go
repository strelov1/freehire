package telegramnotify

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestLinkTokens_RoundTrip(t *testing.T) {
	lt := NewLinkTokens("secret", 10*time.Minute)
	tok, err := lt.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := lt.Parse(tok)
	if err != nil || uid != 42 {
		t.Errorf("Parse = %d, %v; want 42, nil", uid, err)
	}
}

func TestLinkTokens_Expired(t *testing.T) {
	lt := NewLinkTokens("secret", -time.Minute) // already expired
	tok, err := lt.Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lt.Parse(tok); !errors.Is(err, jwt.ErrTokenExpired) {
		t.Errorf("Parse(expired) err = %v, want token-expired", err)
	}
}

func TestLinkTokens_WrongSecretRejected(t *testing.T) {
	tok, err := NewLinkTokens("real", time.Minute).Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewLinkTokens("forged", time.Minute).Parse(tok); err == nil {
		t.Error("Parse with wrong secret succeeded, want signature error")
	}
}

func TestLinkTokens_WrongPurposeRejected(t *testing.T) {
	// A token signed with the same secret but lacking the link purpose (as a
	// session JWT would) must not pass as a link token.
	claims := jwt.RegisteredClaims{
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewLinkTokens("secret", time.Minute).Parse(tok); !errors.Is(err, ErrWrongPurpose) {
		t.Errorf("Parse(no purpose) err = %v, want ErrWrongPurpose", err)
	}
}
