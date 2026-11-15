package normalize

import (
	"strings"
	"unicode"
)

// CleanTitle removes the invisible format characters (Unicode category Cf) a source leaves
// inside a job title: the soft hyphen U+00AD, zero-width joiners, the byte-order mark. Each
// splits a word for a matcher that reads word boundaries. The zero-width space is the
// exception: it separates words, so it becomes a space. Visible text is returned unchanged.
func CleanTitle(s string) string {
	if !strings.ContainsFunc(s, isFormat) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '\u200b' {
			// A zero-width space marks a break BETWEEN words, so it stays a boundary.
			return ' '
		}
		if isFormat(r) {
			return -1
		}
		return r
	}, s)
}

func isFormat(r rune) bool { return unicode.Is(unicode.Cf, r) }
