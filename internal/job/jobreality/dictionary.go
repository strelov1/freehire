package jobreality

import "strings"

// evergreenPhrases are curated multi-word phrases that mark a perpetual talent-pool
// listing. They stay specific so a generic "pipeline" in "data pipeline" never trips
// the signal. EN + RU forms, matched case-insensitively as substrings.
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
