package classify

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FoldTerm reduces a requested term to its queue key: lowercase, ё as е, & as "and",
// and every run of other characters as one hyphen.
func FoldTerm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	s = strings.ReplaceAll(s, "&", " and ")
	var b strings.Builder
	hyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			hyphen = false
			continue
		}
		if !hyphen && b.Len() > 0 {
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// maxGapTermChars bounds a requested term, in characters rather than bytes so a
// Cyrillic term gets the same room as a Latin one.
const maxGapTermChars = 80

// GapTerm validates a term a visitor asks the dictionary to add: it returns the
// trimmed term and its fold (the key that makes "Back-End" and "back end" one
// request), and false when the term is too long or folds to nothing.
func GapTerm(raw string) (trimmed, normalized string, ok bool) {
	trimmed = strings.TrimSpace(raw)
	if utf8.RuneCountInString(trimmed) > maxGapTermChars {
		return "", "", false
	}
	normalized = FoldTerm(trimmed)
	if normalized == "" {
		return "", "", false
	}
	return trimmed, normalized, true
}
