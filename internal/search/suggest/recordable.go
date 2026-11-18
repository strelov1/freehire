package suggest

import "strings"

// Bounds on what may be recorded as demand: generous for a graded title (Title has
// already cut qualifiers) but too small for a pasted paragraph.
const (
	maxRecordedRunes = 60
	maxRecordedWords = 8
)

// Recordable reports whether a normalised query is worth storing as demand. The demand
// table takes public input on every search, and a search phrase is short:
//
//   - Privacy: a pasted email, phone number or job description must not be stored.
//   - Growth: the table is keyed by phrase, and the per-caller rate limit bounds neither
//     the aggregate nor a limiter failure (it fails open).
//
// Rejection is silent; the search itself still runs.
func Recordable(normalised string) bool {
	if normalised == "" {
		return false
	}
	if len([]rune(normalised)) > maxRecordedRunes {
		return false
	}
	if len(strings.Fields(normalised)) > maxRecordedWords {
		return false
	}
	// No posting title carries `@` and Title keeps it, so a short email address would pass
	// every bound above.
	return !strings.ContainsRune(normalised, '@')
}
