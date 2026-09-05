package username

import "testing"

func TestValid(t *testing.T) {
	valid := []string{
		"ivan-petrov",
		"john123",
		"a1-b2",
		"abc",                            // minimum length (3)
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // maximum length (30)
	}
	for _, s := range valid {
		if err := Valid(s); err != nil {
			t.Errorf("Valid(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"",                                // empty
		"ab",                              // too short
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // too long (31)
		"Ivan",                            // uppercase
		"ivan.petrov",                     // dot not allowed
		"-ivan",                           // leading hyphen
		"ivan-",                           // trailing hyphen
		"ivan--petrov",                    // consecutive hyphens
		"ivan_petrov",                     // underscore not allowed
		"ivan petrov",                     // space not allowed
	}
	for _, s := range invalid {
		if err := Valid(s); err == nil {
			t.Errorf("Valid(%q) = nil, want an error", s)
		}
	}
}

func TestSuggest(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ivan@gmail.com", "ivan"},
		{"Ivan.Petrov@Example.COM", "ivan-petrov"}, // dot becomes a hyphen, lowercased
		{"a+b-c@x.io", "ab-c"},                     // '+' dropped, '-' kept
		{"john_doe@x.io", "johndoe"},               // '_' dropped
		{"  spaced @x.io", "spaced"},               // spaces dropped
		{"señor@x.io", "seor"},                     // non-ascii dropped
		{"!!!@x.io", fallback},                     // sanitizes to nothing -> fallback
		{"nolocalpart", "nolocalpart"},             // no '@' -> whole string
		{"UPPER", "upper"},                         // lowercased
		{"a@x.com", fallback},                      // too short (1 char) -> fallback
		{"ab@x.com", fallback},                     // too short (2 chars) -> fallback
		{"abc@x.com", "abc"},                       // exactly minimum length
		{"a..b@x.com", "a-b"},                      // consecutive dots collapse to one hyphen
		{".john@x.com", "john"},                    // leading dot trimmed after conversion
		{"john.@x.com", "john"},                    // trailing dot trimmed after conversion
	}
	for _, c := range cases {
		if got := Suggest(c.in); got != c.want {
			t.Errorf("Suggest(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSuggestTruncatesLongLocalParts(t *testing.T) {
	local := ""
	for i := 0; i < 40; i++ {
		local += "a"
	}
	got := Suggest(local + "@x.com")
	if len(got) > 30 {
		t.Errorf("Suggest(long local part) = %d chars, want <= 30", len(got))
	}
	if err := Valid(got); err != nil {
		t.Errorf("Suggest(long local part) = %q, not Valid: %v", got, err)
	}
}

func TestCandidate(t *testing.T) {
	cases := []struct {
		base string
		n    int
		want string
	}{
		{"ivan", 0, "ivan"}, // n<=1 is the bare base
		{"ivan", 1, "ivan"},
		{"ivan", 2, "ivan-2"}, // first collision suffix
		{"ivan", 5, "ivan-5"},
	}
	for _, c := range cases {
		if got := Candidate(c.base, c.n); got != c.want {
			t.Errorf("Candidate(%q, %d) = %q, want %q", c.base, c.n, got, c.want)
		}
	}
}

func TestIsReserved(t *testing.T) {
	reserved := []string{"admin", "support", "postmaster", "noreply", "freehire", "official", "staff", "moderator"}
	for _, s := range reserved {
		if !IsReserved(s) {
			t.Errorf("IsReserved(%q) = false, want true", s)
		}
	}

	notReserved := []string{"ivan", "ivan-petrov", "freehire-fan"}
	for _, s := range notReserved {
		if IsReserved(s) {
			t.Errorf("IsReserved(%q) = true, want false", s)
		}
	}
}
