package emailprefs_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/identity/auth"
)

const secret = "test-signing-secret"

// The vocabulary is read from the package, never copied here. A hand-written
// {alerts, activity, news} in the test would agree with a hand-written one in the
// code by construction, which proves they match and not that either is complete —
// a fourth group would round-trip untested and the suite would stay green.
func TestRoundTrip(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	// MaxInt64 pins the width: a future narrowing to int32 fails here rather than
	// silently truncating one unlucky account's id into somebody else's.
	for _, id := range []int64{1, 42, math.MaxInt64} {
		for _, g := range emailprefs.SilenceableGroups() {
			token, err := s.Mint(id, g)
			if err != nil {
				t.Fatalf("Mint(%d, %q): %v", id, g, err)
			}
			userID, group, err := s.Parse(token)
			if err != nil {
				t.Fatalf("Parse(Mint(%d, %q)): %v", id, g, err)
			}
			if userID != id || group != g {
				t.Errorf("round trip gave (%d, %q), want (%d, %q)", userID, group, id, g)
			}
		}
	}
}

// The whole legal argument rests on the link still working when someone finds the
// mail months later: CAN-SPAM requires the opt-out to work for at least 30 days
// after a send, and an unsubscribe link that has expired is a failed unsubscribe.
//
// Today that holds only because nobody wrote a clock into the payload, which would
// survive a well-meant future patch that added one. Determinism is the property no
// expiring or nonce-carrying implementation can fake.
func TestMintIsDeterministicSoTheLinkNeverExpires(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	first, err := s.Mint(42, emailprefs.GroupNews)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	second, err := s.Mint(42, emailprefs.GroupNews)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if first != second {
		t.Errorf("Mint is not deterministic: %q then %q — the token has grown a clock or a nonce", first, second)
	}
}

// The payload is "<id>.<group>", so the split is unambiguous only while no group
// name carries the separator. A future Group("news.weekly") would make two
// different pairs sign the same string.
func TestNoGroupNameCarriesTheSeparator(t *testing.T) {
	for _, g := range append(emailprefs.SilenceableGroups(), emailprefs.GroupEssential) {
		if strings.Contains(string(g), ".") {
			t.Errorf("group %q carries the payload separator; the signed payload is no longer unambiguous", g)
		}
	}
}

// An essential mail has no unsubscribe link, so there is nothing to mint for it.
// Refusing here rather than at render time makes the mistake impossible to ship.
func TestEssentialCannotBeMinted(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	if _, err := s.Mint(42, emailprefs.GroupEssential); !errors.Is(err, emailprefs.ErrCannotMint) {
		t.Fatalf("Mint(_, essential) gave %v; an essential mail must have no unsubscribe token", err)
	}
}

func TestMintRejectsANonPositiveUserID(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	for _, id := range []int64{0, -1} {
		if _, err := s.Mint(id, emailprefs.GroupNews); !errors.Is(err, emailprefs.ErrCannotMint) {
			t.Errorf("Mint(%d, news) gave %v; a token must name a real account", id, err)
		}
	}
}

// A mint failure means OUR code asked for a link it may not have — a bug worth
// paging someone over. A parse failure means a stranger sent junk. A caller
// matching one sentinel must not catch the other, or the page never fires.
func TestMintAndParseFailuresDoNotShareASentinel(t *testing.T) {
	s := emailprefs.NewSigner(secret)
	_, mintErr := s.Mint(42, emailprefs.GroupEssential)
	if errors.Is(mintErr, emailprefs.ErrInvalidToken) {
		t.Error("a Mint failure matches ErrInvalidToken; a bug in our own code would read as routine junk traffic")
	}
	_, _, parseErr := s.Parse("garbage")
	if errors.Is(parseErr, emailprefs.ErrCannotMint) {
		t.Error("a Parse failure matches ErrCannotMint; a stranger's junk would page someone")
	}
}

// The property the design actually claims is mutual: a session token and an
// unsubscribe token must never verify against each other's verifier. The salt is
// what guarantees it; this pins it in both directions rather than trusting that the
// two formats happen not to collide. engage (layer 7) may import identity (3).
func TestSessionAndUnsubscribeTokensNeverCrossVerify(t *testing.T) {
	session := auth.NewIssuer(secret, time.Hour)
	prefs := emailprefs.NewSigner(secret)

	sessionToken, err := session.Issue(42, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, _, err := prefs.Parse(sessionToken); err == nil {
		t.Error("a session token parsed as an unsubscribe token; the two keys are not separated")
	}

	prefsToken, err := prefs.Mint(42, emailprefs.GroupNews)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if _, _, err := session.Parse(prefsToken); err == nil {
		t.Error("an unsubscribe token parsed as a session token; holding one would grant a session")
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
