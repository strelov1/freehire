package telegramnotify

import (
	"errors"
	"regexp"
	"testing"
	"time"
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

// TestLinkTokens_TelegramStartParamSafe guards the bug that broke linking in prod:
// a JWT token exceeded Telegram's deep-link `start` limit (1–64 chars,
// [A-Za-z0-9_-]) and was silently dropped. The token MUST fit that constraint.
func TestLinkTokens_TelegramStartParamSafe(t *testing.T) {
	tok, err := NewLinkTokens("secret", 10*time.Minute).Issue(9223372036854775807) // max int64
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 1 || len(tok) > 64 {
		t.Errorf("token length = %d, want 1..64 (Telegram start-param limit)", len(tok))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(tok) {
		t.Errorf("token %q has chars outside [A-Za-z0-9_-] (Telegram start-param alphabet)", tok)
	}
}

func TestLinkTokens_Expired(t *testing.T) {
	lt := NewLinkTokens("secret", -time.Minute) // already expired
	tok, err := lt.Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lt.Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse(expired) err = %v, want ErrInvalidToken", err)
	}
}

func TestLinkTokens_WrongSecretRejected(t *testing.T) {
	tok, err := NewLinkTokens("real", time.Minute).Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewLinkTokens("forged", time.Minute).Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse with wrong secret err = %v, want ErrInvalidToken", err)
	}
}

func TestLinkTokens_GarbageRejected(t *testing.T) {
	lt := NewLinkTokens("secret", time.Minute)
	for _, bad := range []string{"", "not-base64-!!!", "short", "/start"} {
		if _, err := lt.Parse(bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("Parse(%q) err = %v, want ErrInvalidToken", bad, err)
		}
	}
}
