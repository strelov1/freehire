package linkedinprofile

import "testing"

// The masked inputs below are verbatim from a real public profile fetched on
// 2026-09-03: LinkedIn substitutes an asterisk for every letter and digit it
// withholds and keeps the spacing, so the run reads as a sentence-shaped ghost.
func TestMasked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"a withheld job title", "****** ******** ********", true},
		{"a withheld company name", "**********", true},
		{"a withheld description, with its spacing kept", "* ******** *** ******* ********* ******* ********", true},
		{"a withheld school name", "********** ********* ****** ** *********** *************", true},
		{"a single asterisk", "*", true},
		{"asterisks around punctuation LinkedIn kept", "***, *** — ***", true},

		{"an ordinary title", "Senior Backend Engineer", false},
		{"a real company", "RingCentral", false},
		{"a letter survives the run", "C**", false},
		{"a digit survives the run", "3*3", false},
		{"a footnote marker on real text", "Senior Engineer*", false},
		{"empty", "", false},
		{"blank", "   ", false},
		{"punctuation with no asterisk at all", "—", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := masked(tt.in); got != tt.want {
				t.Errorf("masked(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// value is the boundary every string crosses on its way out of this package: a
// masked run and a blank are both "LinkedIn did not give us this", and the caller
// should not have to tell them apart.
func TestValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"real text is returned trimmed", "  Senior Backend Engineer  ", "Senior Backend Engineer"},
		{"a masked run becomes absent", "****** ******** ********", ""},
		{"a masked run with padding becomes absent", "  **********  ", ""},
		{"blank becomes absent", "   ", ""},
		{"empty stays absent", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := value(tt.in); got != tt.want {
				t.Errorf("value(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
