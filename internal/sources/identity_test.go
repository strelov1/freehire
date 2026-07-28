package sources

import "testing"

func TestNamespaceExternalID(t *testing.T) {
	cases := []struct {
		name  string
		board string
		id    string
		want  string
	}{
		{"boarded platform namespaces by board", "acme", "42", "acme:42"},
		{"boardless source uses empty board prefix", "", "1000166598", ":1000166598"},
	}
	for _, tc := range cases {
		if got := NamespaceExternalID(tc.board, tc.id); got != tc.want {
			t.Errorf("%s: NamespaceExternalID(%q, %q) = %q, want %q", tc.name, tc.board, tc.id, got, tc.want)
		}
	}
}

// A third of the workday board names carry an underscore (Capital_One, RingCentral_Careers),
// which LIKE reads as "any one character" — so the pattern MUST escape it, or one board's
// seen-set would claim a sibling board's postings.
func TestBoardIDPattern(t *testing.T) {
	cases := []struct {
		name  string
		board string
		want  string
	}{
		{"plain board matches only its own namespace", "acme", `acme:%`},
		{"underscore is escaped, not a single-character wildcard", "Capital_One", `Capital\_One:%`},
		{"percent is escaped", "a%b", `a\%b:%`},
		{"backslash is escaped first, so its own escape is not re-read", `a\b`, `a\\b:%`},
	}
	for _, tc := range cases {
		if got := BoardIDPattern(tc.board); got != tc.want {
			t.Errorf("%s: BoardIDPattern(%q) = %q, want %q", tc.name, tc.board, got, tc.want)
		}
	}
}
