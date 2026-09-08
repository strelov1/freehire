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

func TestMintHandle_IsUniquePerCall(t *testing.T) {
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		h, err := MintHandle("Senior Backend Engineer")
		if err != nil {
			t.Fatalf("MintHandle: %v", err)
		}
		if seen[h] {
			t.Fatalf("MintHandle returned %q twice in 64 calls", h)
		}
		seen[h] = true
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
