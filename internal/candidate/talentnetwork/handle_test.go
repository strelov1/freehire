package talentnetwork

import (
	"strings"
	"testing"
)

func TestHandleBase_UsesTheResolvedCategory(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"Senior Backend Engineer", "backend"},
		{"Data Engineer", "data-engineering"},
		{"Lead QA Automation Engineer", "qa"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := HandleBase(c.title); got != c.want {
				t.Errorf("HandleBase(%q) = %q, want %q", c.title, got, c.want)
			}
		})
	}
}

// A title the dictionary cannot resolve must still yield a handle. classify never
// guesses, and a candidate whose title it does not know is not a candidate without a
// URL.
func TestHandleBase_FallsBackWhenNothingResolves(t *testing.T) {
	for _, title := range []string{"", "   ", "Chief Vibes Officer", "работаю"} {
		if got := HandleBase(title); got != neutralBase {
			t.Errorf("HandleBase(%q) = %q, want the neutral base %q", title, got, neutralBase)
		}
	}
}

// The seniority is deliberately NOT part of the base. A handle is frozen at mint, and a
// grade is the part of a title most likely to change — baking it in guarantees every
// promoted member carries a URL that contradicts their own card.
func TestHandleBase_IgnoresSeniority(t *testing.T) {
	junior := HandleBase("Junior Backend Engineer")
	principal := HandleBase("Principal Backend Engineer")
	if junior != principal {
		t.Errorf("HandleBase differs by grade: %q vs %q", junior, principal)
	}
}

// TestMintHandle_IsUniquePerCall checks that the suffix carries real entropy, and
// tolerates the birthday collisions that entropy makes inevitable.
//
// Demanding ZERO collisions in 64 draws is not a stronger test, it is a flaky one. The
// suffix is 4 characters from a 32-character alphabet — 32^4 = 1,048,576 values — so
// 64 draws collide with probability 0.19%, or about one run in 520. That is what it did:
// it failed CI on an unrelated pull request on 2026-09-15 with "MintHandle returned
// backend-y7au twice in 64 calls", and the finding was that the suffix is random, which
// is what it is supposed to be.
//
// Allowing up to two collisions costs almost nothing in sensitivity and removes the
// flake outright:
//
//	P(3 or more collisions in 64 draws) ≈ 1.2e-9, about one run in 845 million
//
// while a suffix that had lost its entropy is still caught with room to spare — drop the
// space to a single byte (256 values) and the expected collision count is 7.9, so the
// assertion fails on essentially every run. A constant suffix yields one distinct value.
func TestMintHandle_IsUniquePerCall(t *testing.T) {
	const (
		draws = 64
		// 62, not 64: see the arithmetic above. Two collisions are luck; three are a bug.
		minDistinct = 62
	)

	seen := make(map[string]bool, draws)
	for i := 0; i < draws; i++ {
		h, err := MintHandle("Senior Backend Engineer")
		if err != nil {
			t.Fatalf("MintHandle: %v", err)
		}
		seen[h] = true
	}

	if len(seen) < minDistinct {
		t.Fatalf("MintHandle produced %d distinct handles in %d calls, want at least %d — "+
			"that is far more collision than %d^%d values can explain, so the suffix has lost its entropy",
			len(seen), draws, minDistinct, len(suffixAlphabet), suffixLen)
	}
}

func TestMintHandle_CarriesTheBaseAndPassesValidation(t *testing.T) {
	h, err := MintHandle("Senior Backend Engineer")
	if err != nil {
		t.Fatalf("MintHandle: %v", err)
	}
	if !strings.HasPrefix(h, "backend-") {
		t.Errorf("MintHandle = %q, want it to start with the category base", h)
	}
	if !ValidHandle(h) {
		t.Errorf("MintHandle produced %q, which ValidHandle rejects — the route would 404 on it", h)
	}
}

// The route reads this before touching the database, so what it accepts decides what
// reaches the query. It must reject anything that is not the shape we mint.
func TestValidHandle(t *testing.T) {
	valid := []string{"backend-7f2a", "data-engineering-0000", "user-abcd"}
	for _, h := range valid {
		if !ValidHandle(h) {
			t.Errorf("ValidHandle(%q) = false, want true", h)
		}
	}
	invalid := []string{
		"",
		"backend",             // no suffix
		"Backend-7f2a",        // uppercase
		"backend_7f2a",        // underscore
		"backend--7f2a",       // doubled hyphen
		"-backend-7f2a",       // leading hyphen
		"backend-7f2a-",       // trailing hyphen
		"backend-7f2a/../etc", // path traversal
		strings.Repeat("a", 8) + "-" + strings.Repeat("b", 64), // over length
	}
	for _, h := range invalid {
		if ValidHandle(h) {
			t.Errorf("ValidHandle(%q) = true, want false", h)
		}
	}
}
