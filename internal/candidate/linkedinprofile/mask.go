package linkedinprofile

import (
	"strings"
	"unicode"
)

// masked reports whether s is one of the placeholder runs LinkedIn serves in place
// of text it withholds from an anonymous reader. It substitutes an asterisk per
// withheld letter and digit and leaves the spacing and punctuation alone, so a
// withheld job title arrives as "****** ******** ********" — sentence-shaped, and
// the same length as the sentence it stands for.
//
// The test is therefore "asterisks where the words would be": at least one
// asterisk, and not one letter or digit anywhere. Keying on the absence of real
// characters rather than on a shape means a run that keeps a comma or an em dash
// is still recognised, and that a genuine string is never mistaken for a ghost as
// long as it says anything at all.
func masked(s string) bool {
	star := false
	for _, r := range s {
		switch {
		case r == '*':
			star = true
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return false
		}
	}
	return star
}

// value is the only way a string leaves this package. A masked run and a blank both
// mean "LinkedIn did not release this", and a caller should never have to tell those
// two apart — nor be trusted to remember to check.
func value(s string) string {
	s = strings.TrimSpace(s)
	if masked(s) {
		return ""
	}
	return s
}
