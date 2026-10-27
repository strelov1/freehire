package skilltag

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"go", "go", true},
		{"Go", "go", true},
		{"golang", "go", true},
		{" Golang ", "go", true},
		{"react.js", "react", true},
		// Reachable only through an acronym tier Canonicalize gates by default: a filter
		// value is a tag, not prose, so it must still resolve.
		{"rag", "rag", true},
		{"golangg", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := Resolve(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Resolve(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// Every canonical the package emits is a value postings carry, so every one must resolve
// to itself — otherwise a filter on a real tag would be reported unknown.
func TestResolveAcceptsEveryCanonical(t *testing.T) {
	for _, c := range Canonicals() {
		if got, ok := Resolve(c); !ok || got != c {
			t.Errorf("Resolve(%q) = %q, %v", c, got, ok)
		}
	}
}
