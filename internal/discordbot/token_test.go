package discordbot

import (
	"errors"
	"testing"
	"time"
)

func TestDiscordLinkTokens_RoundTrip(t *testing.T) {
	lt := NewDiscordLinkTokens("secret", 10*time.Minute)
	tok, err := lt.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := lt.Parse(tok)
	if err != nil || uid != 42 {
		t.Errorf("Parse = %d, %v; want 42, nil", uid, err)
	}
}

func TestDiscordLinkTokens_Expired(t *testing.T) {
	lt := NewDiscordLinkTokens("secret", -time.Minute) // already expired
	tok, err := lt.Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lt.Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse(expired) err = %v, want ErrInvalidToken", err)
	}
}

func TestDiscordLinkTokens_WrongSecretRejected(t *testing.T) {
	tok, err := NewDiscordLinkTokens("real", time.Minute).Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDiscordLinkTokens("forged", time.Minute).Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse with wrong secret err = %v, want ErrInvalidToken", err)
	}
}

func TestDiscordLinkTokens_GarbageRejected(t *testing.T) {
	lt := NewDiscordLinkTokens("secret", time.Minute)
	for _, bad := range []string{"", "not-base64-!!!", "short", "/link"} {
		if _, err := lt.Parse(bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("Parse(%q) err = %v, want ErrInvalidToken", bad, err)
		}
	}
}
