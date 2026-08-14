package linktoken

import (
	"testing"
	"time"
)

func TestTokens_RoundTrip(t *testing.T) {
	tk := New("secret", "purpose-a", 10*time.Minute)
	tok := tk.Issue(42)
	userID, ok := tk.Parse(tok)
	if !ok || userID != 42 {
		t.Errorf("Parse = %d, %v; want 42, true", userID, ok)
	}
}

func TestTokens_Expired(t *testing.T) {
	tk := New("secret", "purpose-a", -time.Minute) // already expired
	tok := tk.Issue(1)
	if _, ok := tk.Parse(tok); ok {
		t.Error("Parse(expired) ok = true, want false")
	}
}

func TestTokens_WrongSecretRejected(t *testing.T) {
	tok := New("real", "purpose-a", time.Minute).Issue(1)
	if _, ok := New("forged", "purpose-a", time.Minute).Parse(tok); ok {
		t.Error("Parse with wrong secret ok = true, want false")
	}
}

// TestTokens_PurposeIsolated guards the domain-separation property both
// telegramnotify.LinkTokens and discordbot.DiscordLinkTokens depend on: a
// token minted for one purpose must not verify under another, even with the
// same secret, so the two callers' tokens can never be cross-accepted.
func TestTokens_PurposeIsolated(t *testing.T) {
	tok := New("secret", "purpose-a", time.Minute).Issue(1)
	if _, ok := New("secret", "purpose-b", time.Minute).Parse(tok); ok {
		t.Error("Parse under a different purpose ok = true, want false")
	}
}

func TestTokens_GarbageRejected(t *testing.T) {
	tk := New("secret", "purpose-a", time.Minute)
	for _, bad := range []string{"", "not-base64-!!!", "short", "/start"} {
		if _, ok := tk.Parse(bad); ok {
			t.Errorf("Parse(%q) ok = true, want false", bad)
		}
	}
}
