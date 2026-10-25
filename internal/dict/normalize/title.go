package normalize

import (
	"strings"
	"unicode"
)

// CleanTitle removes the invisible format characters (Unicode category Cf) a source
// leaves inside a job title: the soft hyphen U+00AD a German employer puts in for
// line breaking (written "Rei-ni-gungs-kraft" with U+00AD for each dash), zero-width spaces and joiners, the
// byte-order mark. None is visible and every one splits a word for a matcher that
// reads word boundaries, so a title carrying them resolved nothing in any dictionary.
// Visible text is returned unchanged.
func CleanTitle(s string) string {
	if !strings.ContainsFunc(s, isFormat) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isFormat(r) {
			return -1
		}
		return r
	}, s)
}

func isFormat(r rune) bool { return unicode.Is(unicode.Cf, r) }
