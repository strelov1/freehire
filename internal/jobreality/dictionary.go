package jobreality

import "strings"

// evergreenPhrases are curated phrases that signal a posting is a perpetual
// talent-pool / pipeline listing rather than a specific opening. They are kept
// specific (multi-word) on purpose so a generic word like "pipeline" in "data
// pipeline" never trips the signal — the dictionary never guesses. EN + RU surface
// forms, matched case-insensitively as substrings.
var evergreenPhrases = []string{
	"always hiring",
	"always looking for",
	"we are always",
	"we're always",
	"talent community",
	"talent pool",
	"talent network",
	"future opportunities",
	"future openings",
	"general application",
	"open application",
	"ongoing recruitment",
	// RU
	"всегда в поиске",
	"постоянно ищем",
	"постоянно в поиске",
	"кадровый резерв",
	"будущие вакансии",
}

// HasEvergreenMarker reports whether the description carries a curated evergreen
// phrase. It emits nothing (false) for text it cannot match — never inferring.
func HasEvergreenMarker(description string) bool {
	lower := strings.ToLower(description)
	for _, phrase := range evergreenPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
