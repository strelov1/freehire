package viewlog

import "strings"

// botMarkers are lowercased substrings of well-known crawler/preview User-Agents.
// Deliberately small: missed bots only inflate the page-view number, so this is a
// light filter, not a blocklist.
//
// The generic "bot" is absent, as in internal/platform/tracerlink's classifier: it
// matches "CUBOT", a phone brand. Generic bot names are caught by botSuffixes.
var botMarkers = []string{
	"crawl", // crawler variants
	"spider",
	"slurp", // Yahoo
	"facebookexternalhit",
	"embedly",
	"prerender",
	"headlesschrome",
}

// botSuffixes catch the general shape of a bot's name — "…bot" followed by a version
// separator, as in "Googlebot/2.1" or "bingbot;" — without claiming every product
// whose name happens to contain those three letters (see botMarkers).
var botSuffixes = []string{"bot/", "bot;", "bot)", "bot ", "bot-"}

// isBot reports whether a User-Agent looks like a known crawler or link-preview
// fetcher. Applied only to page-open signals; API reads are never bot-filtered.
func isBot(ua string) bool {
	lower := strings.ToLower(ua)
	for _, m := range botMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	for _, suffix := range botSuffixes {
		if strings.Contains(lower, suffix) {
			return true
		}
	}
	return strings.HasSuffix(lower, "bot")
}
