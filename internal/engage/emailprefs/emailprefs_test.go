package emailprefs_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

const secret = "test-signing-secret"

// mintableGroups are the three a link may name. essential is deliberately absent:
// an essential mail carries no unsubscribe affordance, so a token for it would be a
// link to a switch that does not exist.
var mintableGroups = []emailprefs.Group{
	emailprefs.GroupAlerts,
	emailprefs.GroupActivity,
	emailprefs.GroupNews,
}

func TestRoundTrip(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	for _, g := range mintableGroups {
		token, err := s.Mint(42, g)
		if err != nil {
			t.Fatalf("Mint(42, %q): %v", g, err)
		}
		userID, group, err := s.Parse(token)
		if err != nil {
			t.Fatalf("Parse(Mint(42, %q)): %v", g, err)
		}
		if userID != 42 || group != g {
			t.Errorf("round trip gave (%d, %q), want (42, %q)", userID, group, g)
		}
	}
}

// An essential mail has no unsubscribe link, so there is nothing to mint for it.
// Refusing here rather than at render time makes the mistake impossible to ship.
func TestEssentialCannotBeMinted(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	if _, err := s.Mint(42, emailprefs.GroupEssential); err == nil {
		t.Fatal("Mint(_, essential) returned no error; an essential mail must have no unsubscribe token")
	}
}

func TestMintRejectsANonPositiveUserID(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	for _, id := range []int64{0, -1} {
		if _, err := s.Mint(id, emailprefs.GroupNews); err == nil {
			t.Errorf("Mint(%d, news) returned no error; a token must name a real account", id)
		}
	}
}

// Every rejection path. Each case is a token a caller might actually be handed —
// a truncated copy-paste, a hand-edited id, a link minted before a secret rotation.
func TestParseRefusesEveryTamperedForm(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	valid, err := s.Mint(42, emailprefs.GroupNews)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	parts := strings.Split(valid, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3 (id.group.signature): %q", len(parts), valid)
	}

	// A signature over the bare secret, with no domain separation. This is the
	// mistake the salt exists to prevent: without it a session-signing key and an
	// unsubscribe-signing key are the same key, and a token from one verifies
	// against the other.
	unsalted := hmac.New(sha256.New, []byte(secret))
	unsalted.Write([]byte(parts[0] + "." + parts[1]))
	unsaltedSig := base64.RawURLEncoding.EncodeToString(unsalted.Sum(nil))

	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no separators", "garbage"},
		{"two parts", parts[0] + "." + parts[1]},
		{"four parts", valid + ".extra"},
		{"non-numeric user id", "abc." + parts[1] + "." + parts[2]},
		{"flipped user id", "43." + parts[1] + "." + parts[2]},
		{"flipped group", parts[0] + "." + string(emailprefs.GroupAlerts) + "." + parts[2]},
		{"unknown group", parts[0] + ".billing." + parts[2]},
		{"essential group", parts[0] + "." + string(emailprefs.GroupEssential) + "." + parts[2]},
		{"truncated signature", valid[:len(valid)-4]},
		{"empty signature", parts[0] + "." + parts[1] + "."},
		{"signature is not base64", parts[0] + "." + parts[1] + ".!!!!"},
		{"signature over the unsalted secret", parts[0] + "." + parts[1] + "." + unsaltedSig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := s.Parse(tc.token); err == nil {
				t.Errorf("Parse(%q) returned no error, want refusal", tc.token)
			}
		})
	}
}

// Rotating the signing secret is the only revocation path a stateless token has,
// so it has to actually revoke.
func TestATokenFromAnotherSecretIsRefused(t *testing.T) {
	minted, err := emailprefs.NewSigner(secret).Mint(42, emailprefs.GroupNews)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, _, err := emailprefs.NewSigner("a-different-secret").Parse(minted); err == nil {
		t.Fatal("a token minted under one secret parsed under another; rotating the secret would revoke nothing")
	}
}

// The caller renders one generic failure for every refusal, so the errors need a
// common sentinel to match on rather than a string comparison per case.
func TestEveryRefusalMatchesTheSentinel(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	for _, token := range []string{"", "garbage", "42.news.wrongsig"} {
		if _, _, err := s.Parse(token); !errors.Is(err, emailprefs.ErrInvalidToken) {
			t.Errorf("Parse(%q) gave %v, want it to match ErrInvalidToken", token, err)
		}
	}
}

// A token is a URL query value. If it needed escaping, every mail would carry a
// link that breaks the first time a client re-encodes it.
func TestTokenIsURLSafe(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	token, err := s.Mint(1234567890, emailprefs.GroupActivity)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	const safe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."
	if i := strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune(safe, r) }); i >= 0 {
		t.Errorf("token %q carries %q at %d, which a URL would have to escape", token, token[i], i)
	}
}
