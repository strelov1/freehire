package viewlog

import "strings"

// botMarkers are lowercased substrings of well-known crawler/preview User-Agents.
// The list is deliberately small and conservative: it catches the high-volume
// crawlers that hit SSR pages for SEO and link previews, not every possible bot.
// Missed bots only inflate the page-view number (a transparency figure), so this
// stays a light filter rather than an exhaustive blocklist.
var botMarkers = []string{
	"bot",   // googlebot, bingbot, twitterbot, ahrefsbot, semrushbot, ...
	"crawl", // crawler variants
	"spider",
	"slurp", // Yahoo
	"facebookexternalhit",
	"embedly",
	"prerender",
	"headlesschrome",
}

// isBot reports whether a User-Agent looks like a known crawler or link-preview
// fetcher. Applied only to page-open signals; API reads are never bot-filtered.
func isBot(ua string) bool {
	lower := strings.ToLower(ua)
	for _, m := range botMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
