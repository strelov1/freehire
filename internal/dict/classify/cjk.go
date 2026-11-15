package classify

import (
	"unicode"
	"unicode/utf8"
)

// titleBoundary is the word-boundary rule every title alias is matched under: wordmatch.UnicodeBoundary with one exception, an alias EDGE that is a Japanese or Chinese character needs no boundary on that side.
//
// Neither language separates words, so a compound arrives glued to whatever qualifies it (シニア+データサイエンティスト). UnicodeBoundary counts kana and kanji as letters, which is right for Latin and Cyrillic titles but would let a CJK alias match only when the employer happened to bracket it.
//
// The exception is per side and keyed on the ALIAS, not the title: "sreエンジニア" still needs a boundary before its Latin "s", so it cannot match inside "presreエンジニア", while its katakana tail may run straight into whatever follows. A one-letter Latin alias is what the boundary exists to stop; a CJK alias in this table is a whole role noun, so substring matching is safe for it and for nothing else.
func titleBoundary(s string, start, end int) bool {
	first, _ := utf8.DecodeRuneInString(s[start:end])
	last, _ := utf8.DecodeLastRuneInString(s[start:end])
	if start > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s[:start])
		if !edgeFree(prev, first) {
			return false
		}
	}
	if end < len(s) {
		next, _ := utf8.DecodeRuneInString(s[end:])
		if !edgeFree(next, last) {
			return false
		}
	}
	return true
}

// edgeFree reports whether an alias edge rune may sit next to the title rune beside it.
func edgeFree(neighbour, edge rune) bool {
	return !isWordRune(neighbour) || isCJK(edge) || isCJK(neighbour)
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// isCJK reports whether r is a kanji/hanzi, hiragana or katakana character, including the
// katakana prolonged-sound mark ー that ends エンジニア-style loanwords.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) || r == 'ー'
}
