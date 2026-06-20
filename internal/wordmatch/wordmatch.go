// Package wordmatch reports whether a term occurs as a standalone token in a
// string. The scan is shared; the notion of a token boundary is supplied by the
// caller (Unicode letters/digits for title classification, ASCII alphanumerics —
// with a leading-dot guard — for skill tags), so the two dictionaries that need
// whole-word matching no longer hand-roll the same loop.
package wordmatch

import (
	"unicode"
	"unicode/utf8"
)

// Boundary reports whether the [start,end) span of s is a standalone term — i.e.
// whether each side is a string edge or a separating character. Implementations
// decide what "separating" means.
type Boundary func(s string, start, end int) bool

// Contains reports whether term occurs in s with a valid boundary on each side.
// An empty term never matches.
func Contains(s, term string, ok Boundary) bool {
	if term == "" {
		return false
	}
	for from := 0; ; {
		i := indexFrom(s, term, from)
		if i < 0 {
			return false
		}
		if ok(s, i, i+len(term)) {
			return true
		}
		from = i + 1
	}
}

// indexFrom returns the index of term in s at or after from, or -1.
func indexFrom(s, term string, from int) int {
	for i := from; i+len(term) <= len(s); i++ {
		if s[i:i+len(term)] == term {
			return i
		}
	}
	return -1
}

// UnicodeBoundary treats Unicode letters and digits as word runes, so Cyrillic
// boundaries are handled like Latin ("lead" does not match inside "leading").
func UnicodeBoundary(s string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); isWordRune(r) {
			return false
		}
	}
	if end < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ASCIIBoundary treats ASCII alphanumerics as word bytes, with one asymmetry: a
// LEADING '.' is not a valid left boundary (the term is the suffix of a larger
// dotted token, e.g. "asp.net" must not match ".net"), while a TRAILING '.' is a
// sentence period and is a valid right boundary ("We use C#.").
func ASCIIBoundary(s string, start, end int) bool {
	if alnumAt(s, start-1) || byteAt(s, start-1) == '.' {
		return false
	}
	return !alnumAt(s, end)
}

func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

func alnumAt(s string, i int) bool {
	c := byteAt(s, i)
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}
