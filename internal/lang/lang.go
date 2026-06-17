// Package lang detects the natural language of a job posting deterministically.
// It is a thin guarded wrapper over whatlanggo: it strips markup, requires a
// minimum amount of text, and trusts only a reliable detection — otherwise it
// emits "" rather than guessing, the same doctrine as internal/location and
// internal/classify.
package lang

import (
	"regexp"
	"strings"
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
	if !info.IsReliable() {
		return ""
	}
	return info.Lang.Iso6391()
}
