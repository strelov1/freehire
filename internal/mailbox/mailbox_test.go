package mailbox

import "testing"

func TestHandle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ivan@gmail.com", "ivan"},
		{"Ivan.Petrov@Example.COM", "ivan.petrov"},
		{"a+b-c@x.io", "ab-c"},         // '+' dropped, '-' kept
		{"john_doe@x.io", "johndoe"},   // '_' dropped
		{"  spaced @x.io", "spaced"},   // spaces dropped
		{"señor@x.io", "seor"},         // non-ascii dropped
		{"!!!@x.io", fallbackHandle},   // sanitizes to nothing -> fallback
		{"nolocalpart", "nolocalpart"}, // no '@' -> whole string
		{"UPPER", "upper"},             // lowercased
	}
	for _, c := range cases {
		if got := Handle(c.in); got != c.want {
			t.Errorf("Handle(%q) = %q, want %q", c.in, got, c.want)
		}
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

func TestAddress(t *testing.T) {
	if got := Address("ivan", 1, "inbox.freehire.dev"); got != "ivan@inbox.freehire.dev" {
		t.Errorf("Address n=1 = %q", got)
	}
	if got := Address("ivan", 3, "inbox.freehire.dev"); got != "ivan-3@inbox.freehire.dev" {
		t.Errorf("Address n=3 = %q", got)
	}
}
