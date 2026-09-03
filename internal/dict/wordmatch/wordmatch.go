// Package wordmatch reports whether a term occurs as a standalone token in a
// string. The scan is shared; the notion of a token boundary is supplied by the
// caller (plain Unicode letters/digits for title classification, the same plus a
// leading dot/hyphen guard for curated technology terms), so the dictionaries that
// need whole-word matching no longer hand-roll the same loop.
package wordmatch

import (
	"strings"
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

// indexFrom returns the index of the leftmost occurrence of term in s at or
// after from, or -1.
func indexFrom(s, term string, from int) int {
	i := strings.Index(s[from:], term)
	if i < 0 {
		return -1
	}
	return from + i
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

// TechTermBoundary is UnicodeBoundary plus one asymmetry the curated technology
// vocabularies need: a LEADING '.' or '-' is not a valid left boundary (the term is
// the suffix of a larger punctuated token — "asp.net" must not match ".net",
// "objective-c" must not match "c developer"), while a TRAILING '.' is a sentence
// period and is a valid right boundary ("We use C#.").
//
// The word test is Unicode, not ASCII, even though every curated spelling is ASCII.
// A byte-level test reads a letter outside ASCII as a separator, which makes a
// curated term a "whole word" inside any accented or non-Latin one — "elk" inside
// the Hungarian "elkészítése", because the byte after it is the first of "é". The
// term being ASCII says nothing about its NEIGHBOURS.
func TechTermBoundary(s string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); r == '.' || r == '-' {
			return false
		}
	}
	return UnicodeBoundary(s, start, end)
}
