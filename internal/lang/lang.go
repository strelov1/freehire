// Package lang detects the natural language of a job posting deterministically.
// It is a thin guarded wrapper over whatlanggo: it strips markup, requires a
// minimum amount of text, and trusts only a reliable detection — otherwise it
// emits "" rather than guessing, the same doctrine as internal/location and
// internal/classify.
package lang

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abadojack/whatlanggo"
)

// minRunes is the smallest cleaned-text length worth detecting. Below it the
// signal is too thin to trust (a one-line title in any language reads as noise),
// so Detect returns "".
const minRunes = 40

// tagPattern strips HTML tags so the detector scores the prose, not the Latin
// markup (descriptions are stored as sanitized HTML; the tag names would bias a
// non-English posting toward "en").
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
	// whatlanggo is over-strict on postings that mix English tech terms, brand
	// names, and code fragments: it leaves ~a third of clearly-English descriptions
	// "unreliable". When its own best guess is already English and the text is
	// Latin-script, trust that weaker signal rather than dropping the language. A
	// genuinely non-English posting scores reliably (German/Portuguese/Russian) and
	// never reaches here, so this only rescues English — it never mislabels another
	// language. When the unreliable guess is not English, we still emit "" (never
	// guess), matching the doctrine of internal/location and internal/classify.
	if info.Lang == whatlanggo.Eng && latinLetterRatio(clean) >= 0.9 {
		return whatlanggo.Eng.Iso6391()
	}
	return ""
}

// latinLetterRatio is the fraction of letters in text that are ASCII A–Z/a–z. It
// gates the English fallback: real English prose is ~all ASCII letters, while a
// non-Latin script (Cyrillic, CJK) scores low and is never coerced to English.
// Non-letter runes (digits, punctuation, spaces) are ignored so tech-token-heavy
// text is judged on its words, not its symbols. Returns 0 when there are no letters.
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
