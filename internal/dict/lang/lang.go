// Package lang detects the natural language of a job posting deterministically.
// It is a guarded wrapper over whatlanggo: it strips markup, requires a minimum
// amount of text, and trusts only a reliable detection, else emits "" (never
// guesses), as in internal/dict/location and internal/dict/classify.
package lang

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abadojack/whatlanggo"
)

// minRunes is the smallest cleaned-text length worth detecting; shorter text (a
// one-line title) is too thin to trust, so Detect returns "".
const minRunes = 40

// tagPattern strips HTML tags so Latin markup (descriptions are stored as HTML)
// does not bias a non-English posting toward "en".
var tagPattern = regexp.MustCompile(`<[^>]+>`)

// Detect returns the ISO 639-1 code of text's dominant language, or "" when the
// text is too short or the detection is unreliable. The code is lowercase
// (e.g. "en", "pt", "ru", "uk").
func Detect(text string) string {
	clean := strings.TrimSpace(tagPattern.ReplaceAllString(text, " "))
	if utf8.RuneCountInString(clean) < minRunes {
		return ""
	}
	info := whatlanggo.Detect(clean)
	if info.IsReliable() {
		return info.Lang.Iso6391()
	}
	// whatlanggo marks many English postings "unreliable" when they mix tech terms,
	// brand names and code. Trust its weaker signal only when the best guess is
	// English and the text is Latin-script; any other unreliable guess stays "".
	if info.Lang == whatlanggo.Eng && latinLetterRatio(clean) >= 0.9 {
		return whatlanggo.Eng.Iso6391()
	}
	return ""
}

// latinLetterRatio is the fraction of letters in text that are ASCII A–Z/a–z. It
// gates the English fallback so non-Latin scripts (Cyrillic, CJK) are never coerced
// to English. Non-letters are ignored; returns 0 when there are no letters.
func latinLetterRatio(text string) float64 {
	var letters, ascii int
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			ascii++
		}
	}
	if letters == 0 {
		return 0
	}
	return float64(ascii) / float64(letters)
}
